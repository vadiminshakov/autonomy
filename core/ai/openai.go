package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	openai "github.com/sashabaranov/go-openai"

	"github.com/vadiminshakov/autonomy/core/config"
	"github.com/vadiminshakov/autonomy/core/entity"
	mcpClient "github.com/vadiminshakov/autonomy/core/mcp/client"
	"github.com/vadiminshakov/autonomy/pkg/retry"
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
		if msg.Content == "" && len(msg.ToolCalls) == 0 {
			continue
		}

		var role string
		switch msg.Role {
		case "user":
			role = openai.ChatMessageRoleUser
		case "assistant":
			role = openai.ChatMessageRoleAssistant
		case "tool":
			role = openai.ChatMessageRoleTool
		default:
			role = openai.ChatMessageRoleUser
		}

		// handle assistant messages with tool calls
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			var toolCalls []openai.ToolCall
			for i, tc := range msg.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Args)
				toolCalls = append(toolCalls, openai.ToolCall{
					ID:   fmt.Sprintf("call_%d", i),
					Type: "function",
					Function: openai.FunctionCall{
						Name:      tc.Name,
						Arguments: string(argsJSON),
					},
				})
			}
			
			messages = append(messages, openai.ChatCompletionMessage{
				Role:      role,
				Content:   msg.Content,
				ToolCalls: toolCalls,
			})
		} else if msg.Role == "tool" {
			// handle tool response messages
			messages = append(messages, openai.ChatCompletionMessage{
				Role:       role,
				Content:    msg.Content,
				ToolCallID: msg.ToolCallID,
			})
		} else {
			// handle regular messages
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    role,
				Content: msg.Content,
			})
		}
	}

	return messages
}

func NewOpenAI(cfg config.Config) (*OpenAIClient, error) {
	clientConfig := openai.DefaultConfig(cfg.APIKey)
	clientConfig.BaseURL = cfg.BaseURL

	client := openai.NewClientWithConfig(clientConfig)

	return &OpenAIClient{
		client: client,
		model:  cfg.Model,
	}, nil
}

func (o *OpenAIClient) GenerateCode(ctx context.Context, promptData entity.PromptData) (*entity.AIResponse, error) {
	return o.generateCodeDirect(ctx, promptData)
}

func (o *OpenAIClient) generateCodeDirect(ctx context.Context, promptData entity.PromptData) (*entity.AIResponse, error) {
	formatter := &OpenAIFormatter{}
	messages := formatter.FormatPrompt(&promptData)

	// attach structured tool definitions
	tools := convertTools(promptData.Tools)

	model := o.model
	if model == "" {
		model = "o3"
	}

	// determine the last user message for analysis
	var lastUserMessage string
	for i := len(promptData.Messages) - 1; i >= 0; i-- {
		if promptData.Messages[i].Role == "user" && promptData.Messages[i].Content != "" {
			lastUserMessage = promptData.Messages[i].Content
			break
		}
	}

	req := openai.ChatCompletionRequest{
		Model:     model,
		Messages:  messages,
		Tools:     tools,
		MaxTokens: 16000,
	}

	if len(tools) > 0 {
		toolChoiceMode := DetermineToolChoiceMode(lastUserMessage)
		toolChoice := convertToOpenAIToolChoice(toolChoiceMode)
		req.ToolChoice = toolChoice
		
		// Debug logging for tool choice decisions
		log.Printf("OpenAI API: Tool choice mode=%d, choice=%v, last_user_msg='%s'",
			toolChoiceMode, toolChoice, lastUserMessage)
	}

	var resp openai.ChatCompletionResponse
	var err error

	if err = retry.Exponential(ctx, func() error {
		resp, err = o.client.CreateChatCompletion(ctx, req)
		return err
	}, func(e error) bool {
		apiErr, ok := e.(*openai.APIError)
		return ok && apiErr.HTTPStatusCode == http.StatusTooManyRequests
	}); err != nil {
		// try JSON fallback for HTTP 400
		if apiErr, ok := err.(*openai.APIError); ok && apiErr.HTTPStatusCode == http.StatusBadRequest {
			// Use MCP format in fallback for models that understand it
			messagesWithMCP := o.buildMCPPrompt(messages, promptData.Tools)
			reqWithoutTools := openai.ChatCompletionRequest{
				Model:     model,
				Messages:  messagesWithMCP,
				MaxTokens: 8000,
			}

			var fallbackResp openai.ChatCompletionResponse

			if err = retry.Exponential(ctx, func() error {
				fallbackResp, err = o.client.CreateChatCompletion(ctx, reqWithoutTools)
				return err
			}, func(e error) bool {
				apiErr, ok := e.(*openai.APIError)
				return ok && apiErr.HTTPStatusCode == http.StatusTooManyRequests
			}); err != nil {
				return nil, err
			}

			return o.parseJSONResponse(fallbackResp)
		}

		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response choices - model: %s, response ID: %s, usage: %+v", 
			model, resp.ID, resp.Usage)
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

// buildMCPPrompt creates a system message with MCP tools information
// that models can understand as available tools in Model Context Protocol format.
// This provides a fallback mechanism for models that understand MCP but don't
// support native OpenAI function calling.
func (o *OpenAIClient) buildMCPPrompt(
	messages []openai.ChatCompletionMessage,
	tools []entity.ToolDefinition,
) []openai.ChatCompletionMessage {
	if len(tools) == 0 {
		return messages
	}

	// convert tools to MCP format
	mcpTools := mcpClient.ConvertToMCPTools(tools)

	// create simple JSON representation
	toolsJSON, err := json.MarshalIndent(mcpTools, "", "  ")
	if err != nil {
		return messages
	}

	// create system message with MCP tools and response format instructions
	fallbackSystemPrompt := fmt.Sprintf(`Available tools in Model Context Protocol format:

%s

When you need to use tools, respond with a JSON object in this exact format:
{
  "content": "your response text here",
  "tool_calls": [
    {
      "name": "tool_name",
      "args": {"param1": "value1", "param2": "value2"}
    }
  ]
}

If you don't need to use any tools, just respond normally with plain text.`, string(toolsJSON))

	systemMsg := openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: fallbackSystemPrompt,
	}

	newMsgs := []openai.ChatCompletionMessage{systemMsg}
	newMsgs = append(newMsgs, messages...)
	return newMsgs
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

// parseJSONResponse parses the model response for JSON with tool calls
func (o *OpenAIClient) parseJSONResponse(resp openai.ChatCompletionResponse) (*entity.AIResponse, error) {
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response choices in parseJSONResponse - response ID: %s, usage: %+v", 
			resp.ID, resp.Usage)
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

// convertToOpenAIToolChoice converts general tool choice mode to OpenAI format
func convertToOpenAIToolChoice(mode ToolChoiceMode) interface{} {
	switch mode {
	case ToolChoiceModeAuto:
		return "auto"
	case ToolChoiceModeAny:
		return "required"  // OpenAI uses "required" to force tool usage
	default:
		return "auto"
	}
}
