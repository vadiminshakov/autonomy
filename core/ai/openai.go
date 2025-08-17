package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sashabaranov/go-openai"
	"github.com/vadiminshakov/autonomy/core/config"
	"github.com/vadiminshakov/autonomy/core/entity"
)

type OpenAICompatibleHandler struct {
	*BaseProvider
	client       *openai.Client
	config       config.Config
	providerName string
}

func NewOpenAICompatibleProvider(cfg config.Config, providerName string) *OpenAICompatibleHandler {
	var providerType ProviderType
	switch strings.ToLower(providerName) {
	case "openai":
		providerType = ProviderTypeOpenAI
	case "openrouter":
		providerType = ProviderTypeOpenRouter
	case "groq":
		providerType = ProviderTypeGroq
	case "deepseek":
		providerType = ProviderTypeDeepSeek
	case "local":
		providerType = ProviderTypeLocal
	default:
		providerType = ProviderTypeOpenAI
	}

	capabilities := ProviderCapabilities{
		Tools:         true,
		Images:        providerType == ProviderTypeOpenAI,
		SystemPrompts: true,
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		switch providerType {
		case ProviderTypeOpenAI:
			baseURL = "https://api.openai.com/v1"
		case ProviderTypeOpenRouter:
			baseURL = "https://openrouter.ai/api/v1"
		case ProviderTypeGroq:
			baseURL = "https://api.groq.com/openai/v1"
		case ProviderTypeDeepSeek:
			baseURL = "https://api.deepseek.com"
		}
	}

	baseProvider := NewRouterProvider(providerType, capabilities, baseURL, cfg.APIKey, cfg.Model)

	clientConfig := openai.DefaultConfig(cfg.APIKey)
	if baseURL != "" {
		clientConfig.BaseURL = baseURL
	}

	return &OpenAICompatibleHandler{
		BaseProvider: baseProvider,
		client:       openai.NewClientWithConfig(clientConfig),
		config:       cfg,
		providerName: providerName,
	}
}

// extractJSONFromText извлекает первый JSON объект из текста
func extractJSONFromText(s string) string {
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return ""
}

// parseTextToolCalls parses text format of tool calls
// Format: "Tool call: tool_name (param1: value1, param2: value2)"
func parseTextToolCalls(s string) ([]entity.ToolCall, bool) {
	var calls []entity.ToolCall
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Tool call: ") {
			// extract tool name and parameters
			parts := strings.SplitN(line[11:], " (", 2)
			if len(parts) > 0 {
				toolName := strings.TrimSpace(parts[0])
				args := make(map[string]interface{})

				if len(parts) == 2 && strings.HasSuffix(parts[1], ")") {
					// parse parameters
					paramStr := parts[1][:len(parts[1])-1]
					// for attempt_completion special case
					if toolName == "attempt_completion" && strings.HasPrefix(paramStr, "result: ") {
						args["result"] = strings.TrimPrefix(paramStr, "result: ")
					} else {
						// parse parameters
						params := strings.Split(paramStr, ", ")
						for _, param := range params {
							kv := strings.SplitN(param, ": ", 2)
							if len(kv) == 2 {
								args[kv[0]] = kv[1]
							}
						}
					}
				}

				calls = append(calls, entity.ToolCall{
					Name: toolName,
					Args: args,
				})
			}
		}
	}
	return calls, len(calls) > 0
}

// parseJSONToolCalls парсит JSON формат вызова инструментов
func parseJSONToolCalls(jsonStr string) ([]entity.ToolCall, string, bool) {
	var parsed struct {
		Content   string            `json:"content"`
		ToolCalls []entity.ToolCall `json:"tool_calls"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &parsed); err == nil && len(parsed.ToolCalls) > 0 {
		for i := range parsed.ToolCalls {
			if parsed.ToolCalls[i].Name == "" {
				parsed.ToolCalls[i].Name = parsed.ToolCalls[i].Function.Name
			}
			if parsed.ToolCalls[i].Arguments == "" && parsed.ToolCalls[i].Function.Arguments != "" {
				parsed.ToolCalls[i].Arguments = parsed.ToolCalls[i].Function.Arguments
			}
		}
		return parsed.ToolCalls, parsed.Content, true
	}
	return nil, "", false
}

func (h *OpenAICompatibleHandler) GenerateCode(ctx context.Context, promptData entity.PromptData) (*entity.AIResponse, error) {
	systemPrompt := promptData.SystemPrompt
	if len(promptData.Tools) > 0 {
		systemPrompt += "\n\nyou can and should use tools to complete the task. " +
			"when you decide to use a tool, respond with a json object with a 'tool_calls' array. " +
			"each entry must have { name, args }. example: {\\n  \"tool_calls\": [ { \"name\": \"search_dir\", \"args\": { \"pattern\": \"TODO\" } } ]\\n}\\n" +
			"if no tools are needed, just respond with plain text."
	}

	promptText := systemPrompt + "\n\n"
	for _, msg := range promptData.Messages {
		switch msg.Role {
		case "user":
			promptText += fmt.Sprintf("%s\n", msg.Content)
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				toolCallsJSON := map[string]interface{}{
					"tool_calls": msg.ToolCalls,
				}
				if msg.Content != "" {
					toolCallsJSON["content"] = msg.Content
				}
				jsonBytes, _ := json.Marshal(toolCallsJSON)
				promptText += fmt.Sprintf("Assistant: %s\n", string(jsonBytes))
			} else {
				promptText += fmt.Sprintf("%s\n", msg.Content)
			}
		case "tool":
			promptText += fmt.Sprintf("Tool result: %s\n", msg.Content)
		default:
			promptText += fmt.Sprintf("%s: %s\n", msg.Role, msg.Content)
		}
	}

	response, err := h.CompletePrompt(ctx, promptText)
	if err != nil {
		return nil, err
	}

	// try to parse as JSON
	if calls, content, ok := parseJSONToolCalls(response); ok {
		return &entity.AIResponse{Content: content, ToolCalls: calls}, nil
	}

	// try to extract JSON from text
	if jsonStr := extractJSONFromText(response); jsonStr != "" {
		if calls, content, ok := parseJSONToolCalls(jsonStr); ok {
			return &entity.AIResponse{Content: content, ToolCalls: calls}, nil
		}
	}

	// try to parse text format
	if calls, ok := parseTextToolCalls(response); ok {
		// remove lines with tool calls from content for better readability
		cleanContent := response
		for _, line := range strings.Split(response, "\n") {
			if strings.HasPrefix(line, "Tool call: ") {
				cleanContent = strings.Replace(cleanContent, line+"\n", "", 1)
				cleanContent = strings.Replace(cleanContent, line, "", 1)
			}
		}
		return &entity.AIResponse{Content: strings.TrimSpace(cleanContent), ToolCalls: calls}, nil
	}

	return &entity.AIResponse{Content: response}, nil
}

func (h *OpenAICompatibleHandler) CompletePrompt(ctx context.Context, prompt string) (string, error) {
	if err := h.validateContext(ctx); err != nil {
		return "", err
	}

	modelInfo := h.GetModel()

	req := openai.ChatCompletionRequest{
		Model: modelInfo.ID,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
		MaxTokens:   modelInfo.MaxTokens,
		Temperature: float32(modelInfo.Temperature),
	}

	resp, err := h.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("%s completion error: %w", h.providerName, err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("%s returned no choices", h.providerName)
	}

	return resp.Choices[0].Message.Content, nil
}

func (h *OpenAICompatibleHandler) GetModel() ModelInfo {
	modelID := h.config.Model
	if modelID == "" {
		modelID = h.defaultModel
	}

	maxTokens := 4096
	if h.config.MaxTokens > 0 {
		maxTokens = h.config.MaxTokens
	}

	temperature := 1.0
	if h.config.Temperature >= 0 {
		temperature = h.config.Temperature
	}

	return ModelInfo{
		ID:                   modelID,
		MaxTokens:            maxTokens,
		Temperature:          temperature,
		ContextWindow:        128000,
		SupportsImages:       h.GetCapabilities().Images,
		SupportsTools:        h.GetCapabilities().Tools,
		SupportsSystemPrompt: h.GetCapabilities().SystemPrompts,
		Description:          fmt.Sprintf("%s model", h.providerName),
	}
}
