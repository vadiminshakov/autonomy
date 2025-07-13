package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"autonomy/internal/pkg/task"

	"github.com/sashabaranov/go-openai"
)

type OpenAIClient struct {
	client *openai.Client
}

// OpenAIFormatter formats prompts for OpenAI's chat completion API
type OpenAIFormatter struct{}

func (f *OpenAIFormatter) FormatPrompt(data *task.PromptData) []openai.ChatCompletionMessage {
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

func convertTools(tools []task.ToolDefinition) []openai.FunctionDefinition {
	defs := make([]openai.FunctionDefinition, 0, len(tools))
	for _, t := range tools {
		// marshal the input schema as-is – OpenAI expects JSON schema
		params, _ := json.Marshal(t.InputSchema)

		defs = append(defs, openai.FunctionDefinition{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  json.RawMessage(params),
		})
	}
	return defs
}

func (o *OpenAIClient) GenerateCode(promptData *task.PromptData) (string, error) {
	return o.GenerateCodeWithContext(context.Background(), promptData)
}

func (o *OpenAIClient) GenerateCodeWithContext(ctx context.Context, promptData *task.PromptData) (string, error) {
	formatter := &OpenAIFormatter{}
	messages := formatter.FormatPrompt(promptData)

	// attach structured tool definitions
	functions := convertTools(promptData.Tools)

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
			MaxTokens:    4096,
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

		// 1. Function call path (structured tool)
		if choice.Message.FunctionCall != nil && choice.Message.FunctionCall.Name != "" {
			name := choice.Message.FunctionCall.Name
			var argObj map[string]interface{}
			if err := json.Unmarshal([]byte(choice.Message.FunctionCall.Arguments), &argObj); err != nil {
				// if parsing fails, return raw args string for debugging
				argObj = map[string]interface{}{"raw": choice.Message.FunctionCall.Arguments}
			}

			// build markdown block compatible with existing parser
			md := "```" + name + "\n"
			for k, v := range argObj {
				md += fmt.Sprintf("%s: %v\n", k, v)
			}
			md += "```"

			return md, nil
		}

		// 2. Plain text path (no function invoked)
		return choice.Message.Content, nil
	}

	return "", fmt.Errorf("all OpenAI models are unavailable, last error: %v", lastErr)
}
