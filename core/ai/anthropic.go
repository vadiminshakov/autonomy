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
//
//nolint:gocyclo
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

	params := anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: 16000,
		Messages:  msgs,
		Tools:     anthropicTools,
		System: []anthropic.TextBlockParam{{
			Text: pd.SystemPrompt,
			Type: constant.Text("text"),
		}},
	}

	if len(anthropicTools) > 0 {
		toolChoiceMode := DetermineToolChoiceMode(lastUserMessage)
		toolChoice := convertToAnthropicToolChoice(toolChoiceMode)
		params.ToolChoice = toolChoice
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

// convertToAnthropicToolChoice конвертирует общий режим выбора инструментов в формат Anthropic
func convertToAnthropicToolChoice(mode ToolChoiceMode) anthropic.ToolChoiceUnionParam {
	switch mode {
	case ToolChoiceModeAuto:
		return anthropic.ToolChoiceUnionParam{
			OfAuto: &anthropic.ToolChoiceAutoParam{Type: constant.Auto("auto")},
		}
	case ToolChoiceModeAny:
		return anthropic.ToolChoiceUnionParam{
			OfAny: &anthropic.ToolChoiceAnyParam{Type: constant.Any("any")},
		}
	default:
		return anthropic.ToolChoiceUnionParam{
			OfAny: &anthropic.ToolChoiceAnyParam{Type: constant.Any("any")},
		}
	}
}
