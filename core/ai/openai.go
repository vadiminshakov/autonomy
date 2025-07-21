package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/pkg/errors"
	openai "github.com/sashabaranov/go-openai"

	"github.com/vadiminshakov/autonomy/core/config"
	"github.com/vadiminshakov/autonomy/core/entity"
)

type OpenAIClient struct {
	client *openai.Client
	model  string
}

// OpenAIFormatter formats prompts for OpenAI's chat completion API
type OpenAIFormatter struct{}

func (f *OpenAIFormatter) FormatPrompt(data *entity.PromptData) []openai.ChatCompletionMessage {
	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: data.SystemPrompt,
		},
	}

	for _, msg := range data.Messages {
		if msg.Content == "" {
			continue
		}

		var role string
		switch msg.Role {
		case "user":
			role = openai.ChatMessageRoleUser
		case "assistant":
			role = openai.ChatMessageRoleAssistant
		default:
			role = openai.ChatMessageRoleUser
		}

		messages = append(messages, openai.ChatCompletionMessage{
			Role:    role,
			Content: msg.Content,
		})
	}

	return messages
}

func NewOpenai(cfg config.Config) (*OpenAIClient, error) {
	clientConfig := openai.DefaultConfig(cfg.APIKey)
	clientConfig.BaseURL = cfg.BaseURL

	client := openai.NewClientWithConfig(clientConfig)

	model := cfg.Model

	return &OpenAIClient{
		client: client,
		model:  model,
	}, nil
}

func convertTools(tools []entity.ToolDefinition) []openai.Tool {
	defs := make([]openai.Tool, 0, len(tools))
	for _, t := range tools {
		params, _ := json.Marshal(t.InputSchema)

		defs = append(defs, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  json.RawMessage(params),
			},
		})
	}
	return defs
}

// GenerateCode generates AI response using OpenAI API
func (o *OpenAIClient) GenerateCode(ctx context.Context, promptData entity.PromptData) (*entity.AIResponse, error) {
	formatter := &OpenAIFormatter{}
	messages := formatter.FormatPrompt(&promptData)

	// attach structured tool definitions
	tools := convertTools(promptData.Tools)

	model := o.model
	if model == "" {
		model = "o3"
	}

	req := openai.ChatCompletionRequest{
		Model:      model,
		Messages:   messages,
		Tools:      tools,
		ToolChoice: "auto",
		MaxTokens:  8000,
	}

	resp, err := o.client.CreateChatCompletion(ctx, req)
	if err != nil {
		// try JSON fallback for HTTP 400
		if apiErr, ok := err.(*openai.APIError); ok && apiErr.HTTPStatusCode == http.StatusBadRequest {
			messagesWithToolsInJSON := o.addToolsToSystemPrompt(messages, promptData.Tools)
			reqWithoutTools := openai.ChatCompletionRequest{
				Model:     model,
				Messages:  messagesWithToolsInJSON,
				MaxTokens: 8000,
			}

			resp, err = o.client.CreateChatCompletion(ctx, reqWithoutTools)
			if err != nil {
				return nil, err
			}

			return o.parseJSONResponse(resp)
		}

		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, errors.New("no response choices")
	}

	choice := resp.Choices[0]

	if len(choice.Message.ToolCalls) > 0 {
		var toolCalls []entity.ToolCall
		for _, tc := range choice.Message.ToolCalls {
			var argObj map[string]any
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &argObj); err != nil {
				argObj = map[string]any{"raw": tc.Function.Arguments}
			}

			toolCalls = append(toolCalls, entity.ToolCall{
				Name: tc.Function.Name,
				Args: argObj,
			})
		}

		return &entity.AIResponse{
			Content:   choice.Message.Content,
			ToolCalls: toolCalls,
		}, nil
	}

	return &entity.AIResponse{
		Content: choice.Message.Content,
	}, nil
}

// addToolsToSystemPrompt adds tool descriptions to the system prompt for models that don't support tools
func (o *OpenAIClient) addToolsToSystemPrompt(
	messages []openai.ChatCompletionMessage,
	tools []entity.ToolDefinition,
) []openai.ChatCompletionMessage {
	// build strict JSON-only instructions
	strictPromptHeader := `You are a tool-using AI assistant. Read carefully and STRICTLY follow every rule.

WHEN A TOOL IS NEEDED
• Output STRICT JSON ONLY (no markdown, no plaintext outside the JSON).
• Required schema:
  {
    "content": "<short explanation>",
    "tool_calls": [
      { "name": "<toolName>", "args": { "param": "value" } }
    ]
  }
• If a tool takes no arguments, pass an empty object: "args": {}.
• Multiple tools → put several objects in the same "tool_calls" array.
• "content" is mandatory (may be an empty string if nothing to add).

WHEN NO TOOL IS REQUIRED
• Reply with JSON:
  { "content": "<answer>" }

ABSOLUTE PROHIBITIONS
• Never output anything outside the JSON block (even greetings or explanations).
• Never invent tool names – use only those from the provided list below.
• Ensure the JSON is syntactically valid (matched braces, double quotes).

Failure to comply will be treated as a fatal error.`

	// include JSON description for each tool (name, description, schema)
	var toolJSON []string
	for _, t := range tools {
		b, err := json.Marshal(t)
		if err != nil {
			continue
		}

		toolJSON = append(toolJSON, string(b))
	}

	toolsList := strings.Join(toolJSON, "\n")

	strictPrompt := fmt.Sprintf("%s\n\nAvailable tools (JSON):\n%s", strictPromptHeader, toolsList)

	newMessages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: strictPrompt,
		},
	}
	newMessages = append(newMessages, messages...)
	return newMessages
}

// parseJSONResponse parses the model response for JSON with tool calls
func (o *OpenAIClient) parseJSONResponse(resp openai.ChatCompletionResponse) (*entity.AIResponse, error) {
	if len(resp.Choices) == 0 {
		return nil, errors.New("no response choices")
	}

	choice := resp.Choices[0]
	content := choice.Message.Content

	var jsonResponse struct {
		Content   string `json:"content"`
		ToolCalls []struct {
			Name string                 `json:"name"`
			Args map[string]interface{} `json:"args"`
		} `json:"tool_calls"`
	}

	// try to find JSON block in the response
	jsonStart := strings.Index(content, "{")
	jsonEnd := strings.LastIndex(content, "}")

	if jsonStart != -1 && jsonEnd != -1 && jsonEnd > jsonStart {
		jsonStr := content[jsonStart : jsonEnd+1]
		if err := json.Unmarshal([]byte(jsonStr), &jsonResponse); err == nil {
			// successfully parsed JSON with tool calls
			if len(jsonResponse.ToolCalls) > 0 {
				var toolCalls []entity.ToolCall
				for _, tc := range jsonResponse.ToolCalls {
					toolCalls = append(toolCalls, entity.ToolCall{
						Name: tc.Name,
						Args: tc.Args,
					})
				}

				return &entity.AIResponse{
					Content:   jsonResponse.Content,
					ToolCalls: toolCalls,
				}, nil
			}
		}
	}

	// if we couldn't find JSON with tool calls, return as a regular response
	return &entity.AIResponse{
		Content: content,
	}, nil
}
