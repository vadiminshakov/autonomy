package ai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sashabaranov/go-openai"
	"github.com/vadiminshakov/autonomy/core/entity"
)

func ConvertToOpenAIMessages(messages []entity.Message) []openai.ChatCompletionMessage {
	var openAIMessages []openai.ChatCompletionMessage

	for _, msg := range messages {
		role := openai.ChatMessageRoleUser
		switch strings.ToLower(msg.Role) {
		case "user":
			role = openai.ChatMessageRoleUser
		case "assistant":
			role = openai.ChatMessageRoleAssistant
		case "system":
			role = openai.ChatMessageRoleSystem
		case "tool":
			role = openai.ChatMessageRoleTool
		}

		if len(msg.ToolCalls) > 0 {
			var toolCalls []openai.ToolCall
			for _, tc := range msg.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Arguments)
				toolCalls = append(toolCalls, openai.ToolCall{
					ID:   tc.ID,
					Type: openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name:      tc.Function.Name,
						Arguments: string(argsJSON),
					},
				})
			}
			openAIMessages = append(openAIMessages, openai.ChatCompletionMessage{
				Role:      role,
				Content:   msg.Content,
				ToolCalls: toolCalls,
			})
		} else if msg.ToolCallID != "" {
			openAIMessages = append(openAIMessages, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				Content:    msg.Content,
				ToolCallID: msg.ToolCallID,
			})
		} else {
			openAIMessages = append(openAIMessages, openai.ChatCompletionMessage{
				Role:    role,
				Content: msg.Content,
			})
		}
	}

	return openAIMessages
}

func isAnthropicModel(modelID string) bool {
	return strings.HasPrefix(modelID, "anthropic/")
}

func SupportsNativeTools(modelID string) bool {
	if isAnthropicModel(modelID) {
		return false
	}
	unsupportedModels := []string{"o1", "o1-mini"}
	for _, unsupported := range unsupportedModels {
		if modelID == unsupported || strings.HasPrefix(modelID, unsupported+"-") {
			return false
		}
	}
	return true
}

func BuildToolsSystemMessage(tools []entity.ToolDefinition) string {
	if len(tools) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Available tools:\n\n")

	for _, tool := range tools {
		sb.WriteString(fmt.Sprintf("**%s**: %s\n", tool.Name, tool.Description))
		if tool.InputSchema != nil {
			if props, ok := tool.InputSchema["properties"].(map[string]interface{}); ok {
				sb.WriteString("Parameters:\n")
				for paramName, paramInfo := range props {
					if param, ok := paramInfo.(map[string]interface{}); ok {
						if desc, ok := param["description"].(string); ok {
							sb.WriteString(fmt.Sprintf("- %s: %s\n", paramName, desc))
						}
					}
				}
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("To use a tool, respond with JSON in this format:\n")
	sb.WriteString("```json\n{\n")
	sb.WriteString(`  "content": "your response text",` + "\n")
	sb.WriteString(`  "tool_calls": [{"name": "tool_name", "args": {"param1": "value1"}}]` + "\n")
	sb.WriteString("}\n```")

	return sb.String()
}
