package ai

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/pkg/errors"

	"github.com/vadiminshakov/autonomy/core/config"
	"github.com/vadiminshakov/autonomy/core/entity"
	"github.com/vadiminshakov/autonomy/pkg/retry"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
)

type AnthropicClient struct {
	client *anthropic.Client
	model  anthropic.Model
}

// NewAnthropic constructs a client with provided configuration
func NewAnthropic(cfg config.Config) (*AnthropicClient, error) {
	options := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}

	if cfg.BaseURL != "" {
		options = append(options, option.WithBaseURL(cfg.BaseURL))
	}

	cli := anthropic.NewClient(options...)

	var model anthropic.Model
	if cfg.Model != "" {
		model = anthropic.Model(cfg.Model)
	}

	return &AnthropicClient{client: &cli, model: model}, nil
}

// GenerateCode generates AI response using Anthropic API
func (c *AnthropicClient) GenerateCode(ctx context.Context, pd entity.PromptData) (*entity.AIResponse, error) {
	// build message list from conversation history
	var msgs []anthropic.MessageParam
	var lastUserMessage string

	for _, m := range pd.Messages {
		// skip empty messages to avoid Anthropic API error
		if m.Content == "" {
			continue
		}

		content := anthropic.NewTextBlock(m.Content)

		switch m.Role {
		case "user":
			msgs = append(msgs, anthropic.NewUserMessage(content))
			lastUserMessage = m.Content
		case "assistant":
			msgs = append(msgs, anthropic.NewAssistantMessage(content))
		}
	}

	// convert tools provided in PromptData into Anthropic tool definitions
	var anthropicTools []anthropic.ToolUnionParam
	for _, td := range pd.Tools {
		var props any
		if p, ok := td.InputSchema["properties"]; ok {
			props = p
		}

		var req []string
		if r, ok := td.InputSchema["required"].([]string); ok {
			req = r
		}

		schema := anthropic.ToolInputSchemaParam{
			Type:       constant.Object("object"),
			Properties: props,
			Required:   req,
		}
		anthropicTools = append(anthropicTools, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        td.Name,
				Description: anthropic.String(td.Description),
				InputSchema: schema,
			},
		})
	}

	model := c.model
	if model == "" {
		model = anthropic.ModelClaude4Sonnet20250514
	}

	toolChoice := determineToolChoice(lastUserMessage)

	params := anthropic.MessageNewParams{
		Model:      model,
		MaxTokens:  8000,
		Messages:   msgs,
		Tools:      anthropicTools,
		ToolChoice: toolChoice,
		System: []anthropic.TextBlockParam{{
			Text: pd.SystemPrompt,
			Type: constant.Text("text"),
		}},
	}

	var resp *anthropic.Message
	var err error
	
	if err = retry.Exponential(ctx, func() error {
		resp, err = c.client.Messages.New(ctx, params)
		return err
	}, func(e error) bool {
		errStr := strings.ToLower(e.Error())
		return strings.Contains(errStr, "429") || strings.Contains(errStr, "rate limit")
	}); err != nil {
		return nil, err
	}

	if len(resp.Content) == 0 {
		return nil, errors.New("anthropic response contained no content")
	}

	var toolCalls []entity.ToolCall
	var textContent string

	for _, blk := range resp.Content {
		switch blk.Type {
		case "text":
			textContent = blk.Text
		case "tool_use":
			var obj map[string]any
			if len(blk.Input) > 0 {
				_ = json.Unmarshal(blk.Input, &obj)
			}
			toolCalls = append(toolCalls, entity.ToolCall{
				Name: blk.Name,
				Args: obj,
			})
		}
	}

	return &entity.AIResponse{
		Content:   textContent,
		ToolCalls: toolCalls,
	}, nil
}

// determineToolChoice analyzes the user message to decide whether to force tool usage
func determineToolChoice(userMessage string) anthropic.ToolChoiceUnionParam {
	// convert to lowercase for case-insensitive matching
	lowerMsg := strings.ToLower(userMessage)

	if isToolResultMessage(lowerMsg) {
		return anthropic.ToolChoiceUnionParam{
			OfAny: &anthropic.ToolChoiceAnyParam{Type: constant.Any("any")},
		}
	}

	msgType := analyzeMessageType(lowerMsg)

	switch msgType {
	case "action":
		// force tool usage for action requests
		return anthropic.ToolChoiceUnionParam{
			OfAny: &anthropic.ToolChoiceAnyParam{Type: constant.Any("any")},
		}
	case "question":
		// let model decide for pure questions
		return anthropic.ToolChoiceUnionParam{
			OfAuto: &anthropic.ToolChoiceAutoParam{Type: constant.Auto("auto")},
		}
	default:
		// when uncertain, prefer tool usage
		return anthropic.ToolChoiceUnionParam{
			OfAny: &anthropic.ToolChoiceAnyParam{Type: constant.Any("any")},
		}
	}
}

// isToolResultMessage checks if the message is a tool execution result
func isToolResultMessage(msg string) bool {
	// Structural patterns that indicate tool results
	patterns := []string{
		`^result of \w+:`,
		`^error executing \w+:`,
		`^✅ done \w+`,
		`^❌ error in \w+`,
		`tools used:`,
		`files created:`,
		`files modified:`,
		`files read:`,
		`commands executed:`,
	}

	for _, pattern := range patterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}

	// check for common result indicators
	resultIndicators := []string{
		"result of ",
		"error executing ",
		"task state summary",
		"✅ done",
		"❌ error",
		"successfully",
		"failed to",
		"completed",
	}

	for _, indicator := range resultIndicators {
		if strings.Contains(msg, indicator) {
			return true
		}
	}

	return false
}

// analyzeMessageType determines if the message is an action request or question
func analyzeMessageType(msg string) string {
	// action keywords that indicate a tool should be used
	actionKeywords := []string{
		// english verbs
		"read", "write", "create", "update", "delete", "execute", "run", "check",
		"analyze", "search", "find", "look", "show", "display", "get", "fetch",
		"test", "build", "compile", "fix", "modify", "edit", "rename", "move",
		"copy", "list", "view", "examine", "inspect", "review", "implement",
		"add", "remove", "install", "configure", "setup", "deploy", "debug",
		// russian verbs
		"прочитай", "читай", "посмотри", "покажи", "найди", "проверь", "запусти",
		"создай", "удали", "измени", "исправь", "проанализируй", "выполни",
		"протестируй", "собери", "скомпилируй", "переименуй", "скопируй",
		"перемести", "отобрази", "изучи", "рассмотри", "реализуй", "добавь",
		"установи", "настрой", "отладь",
		// imperative patterns
		"let's", "давай", "please", "пожалуйста", "can you", "could you",
		"i need", "мне нужно", "необходимо", "требуется", "нужно",
		"make", "do", "perform", "сделай", "выполни",
	}

	// question keywords that typically don't need tools
	questionKeywords := []string{
		"what is", "what are", "what does", "what do",
		"why is", "why are", "why does", "why do",
		"how is", "how are", "how does", "how do",
		"when is", "when are", "when does", "when do",
		"where is", "where are", "where does", "where do",
		"who is", "who are", "which is", "which are",
		"explain", "describe", "tell me about",
		// russian question patterns
		"что такое", "что это", "почему", "как работает",
		"когда", "где", "кто", "какой", "какая", "какие",
		"объясни", "расскажи", "опиши",
	}

	// check for question patterns first (more specific)
	for _, keyword := range questionKeywords {
		if strings.Contains(msg, keyword) {
			// but override if it also contains strong action words
			for _, action := range []string{"implement", "create", "write", "fix", "реализуй", "создай", "напиши", "исправь"} {
				if strings.Contains(msg, action) {
					return "action"
				}
			}

			return "question"
		}
	}

	// check for action keywords
	for _, keyword := range actionKeywords {
		if strings.Contains(msg, keyword) {
			return "action"
		}
	}

	// default to uncertain
	return "uncertain"
}
