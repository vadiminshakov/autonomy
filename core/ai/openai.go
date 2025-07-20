package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/vadiminshakov/autonomy/core/entity"

	openai "github.com/sashabaranov/go-openai"
)

type OpenAIClient struct {
	client *openai.Client
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

func NewOpenai() (*OpenAIClient, error) {
	return &OpenAIClient{
		client: openai.NewClient(""),
	}, nil
}

func convertTools(tools []entity.ToolDefinition) []openai.FunctionDefinition {
	defs := make([]openai.FunctionDefinition, 0, len(tools))
	for _, t := range tools {
		params, _ := json.Marshal(t.InputSchema)

		defs = append(defs, openai.FunctionDefinition{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  json.RawMessage(params),
		})
	}
	return defs
}

// GenerateCode generates AI response using OpenAI API
func (o *OpenAIClient) GenerateCode(ctx context.Context, promptData entity.PromptData) (*entity.AIResponse, error) {
	formatter := &OpenAIFormatter{}
	messages := formatter.FormatPrompt(&promptData)

	// attach structured tool definitions
	functions := convertTools(promptData.Tools)

	// default model list (try the newest first, fall back if unavailable)
	models := []string{
		"gpt-4o",
		"gpt-4o-mini",
		"gpt-4-turbo",
		"gpt-3.5-turbo",
	}

	var lastErr error
	for _, model := range models {
		req := openai.ChatCompletionRequest{
			Model:        model,
			Messages:     messages,
			Functions:    functions,
			FunctionCall: "auto",
			MaxTokens:    8000,
		}

		resp, err := o.client.CreateChatCompletion(ctx, req)
		if err != nil {
			lastErr = err
			continue
		}

		if len(resp.Choices) == 0 {
			lastErr = errors.New("no response choices")
			continue
		}

		choice := resp.Choices[0]

		// Handle function call
		if choice.Message.FunctionCall != nil && choice.Message.FunctionCall.Name != "" {
			name := choice.Message.FunctionCall.Name
			var argObj map[string]any
			if err := json.Unmarshal([]byte(choice.Message.FunctionCall.Arguments), &argObj); err != nil {
				// if parsing fails, return raw args string for debugging
				argObj = map[string]any{"raw": choice.Message.FunctionCall.Arguments}
			}

			return &entity.AIResponse{
				Content: choice.Message.Content,
				ToolCalls: []entity.ToolCall{
					{
						Name: name,
						Args: argObj,
					},
				},
			}, nil
		}

		return &entity.AIResponse{
			Content: choice.Message.Content,
		}, nil
	}

	return nil, fmt.Errorf("all OpenAI models are unavailable, last error: %v", lastErr)
}
