package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"autonomy/core/task"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
)

type AnthropicClient struct {
	client *anthropic.Client
}

// NewAnthropic constructs a client authorised with ANTHROPIC_API_KEY.
func NewAnthropic() (*AnthropicClient, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("environment variable ANTHROPIC_API_KEY is not set")
	}

	cli := anthropic.NewClient(
		option.WithAPIKey(apiKey),
	)

	return &AnthropicClient{client: &cli}, nil
}

// GenerateCode generates AI response using Anthropic API
func (c *AnthropicClient) GenerateCode(pd *task.PromptData) (string, error) {
	return c.GenerateCodeWithContext(context.Background(), pd)
}

// GenerateCodeWithContext converts PromptData into Anthropic Messages API call with context support
func (c *AnthropicClient) GenerateCodeWithContext(ctx context.Context, pd *task.PromptData) (string, error) {
	// Build message list from conversation history
	var msgs []anthropic.MessageParam

	for _, m := range pd.Messages {
		content := anthropic.NewTextBlock(m.Content)

		switch m.Role {
		case "user":
			msgs = append(msgs, anthropic.NewUserMessage(content))
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

	// Default model list (try the newest first, fall back if unavailable)
	models := []anthropic.Model{
		anthropic.ModelClaude3_7SonnetLatest,
		anthropic.ModelClaude3_5SonnetLatest,
	}

	var lastErr error

	for _, model := range models {
		toolChoice := anthropic.ToolChoiceUnionParam{
			OfAuto: &anthropic.ToolChoiceAutoParam{Type: constant.Auto("auto")},
		}

		resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:      model,
			MaxTokens:  int64(4096),
			Messages:   msgs,
			Tools:      anthropicTools,
			ToolChoice: toolChoice,
			System: []anthropic.TextBlockParam{{
				Text: pd.SystemPrompt,
				Type: constant.Text("text"),
			}},
		})
		if err != nil {
			lastErr = err

			continue
		}

		if len(resp.Content) == 0 {
			return "", fmt.Errorf("anthropic response contained no content")
		}

		for _, blk := range resp.Content {
			switch blk.Type {
			case "text":
				return blk.Text, nil
			case "tool_use":
				var markdown string
				markdown += "```" + blk.Name + "\n"
				if len(blk.Input) > 0 {
					var obj map[string]interface{}
					_ = json.Unmarshal(blk.Input, &obj)
					for k, v := range obj {
						markdown += fmt.Sprintf("%s: %v\n", k, v)
					}
				}
				markdown += "```"

				return markdown, nil
			}
		}

		return fmt.Sprintf("%+v", resp.Content[0]), nil
	}

	return "", fmt.Errorf("all anthropic models unavailable, last error: %v", lastErr)
}
