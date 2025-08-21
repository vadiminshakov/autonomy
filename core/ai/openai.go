package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"

	"github.com/vadiminshakov/autonomy/core/config"
	"github.com/vadiminshakov/autonomy/core/entity"
)

type OpenAICompatibleHandler struct {
	providerType ProviderType
	capabilities ProviderCapabilities
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

	clientConfig := openai.DefaultConfig(cfg.APIKey)
	if baseURL != "" {
		clientConfig.BaseURL = baseURL
	}

	return &OpenAICompatibleHandler{
		providerType: providerType,
		capabilities: capabilities,
		client:       openai.NewClientWithConfig(clientConfig),
		config:       cfg,
		providerName: providerName,
	}
}

func (h *OpenAICompatibleHandler) GenerateCode(ctx context.Context, promptData entity.PromptData) (*entity.AIResponse, error) {
	// Use native tool calling for all providers
	return h.generateCodeNative(ctx, promptData)
}

// generateCodeNative uses native OpenAI function calling API
func (h *OpenAICompatibleHandler) generateCodeNative(ctx context.Context, promptData entity.PromptData) (*entity.AIResponse, error) {
	if err := h.validateContext(ctx); err != nil {
		return nil, err
	}

	modelInfo := h.GetModel()

	// Convert messages to OpenAI format
	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: promptData.SystemPrompt},
	}

	// Add conversation messages
	for _, msg := range promptData.Messages {
		openAIMsg := h.convertEntityMessageToOpenAI(msg)
		if openAIMsg != nil {
			messages = append(messages, *openAIMsg)
		}
	}

	req := openai.ChatCompletionRequest{
		Model:       modelInfo.ID,
		Messages:    messages,
		MaxTokens:   modelInfo.MaxTokens,
		Temperature: float32(modelInfo.Temperature),
	}

	// Add tools if available
	if len(promptData.Tools) > 0 {
		var tools []openai.Tool
		for _, tool := range promptData.Tools {
			openAITool := openai.Tool{
				Type: openai.ToolTypeFunction,
				Function: &openai.FunctionDefinition{
					Name:        tool.Name,
					Description: tool.Description,
					Parameters:  tool.InputSchema,
				},
			}
			tools = append(tools, openAITool)
		}
		req.Tools = tools
		req.ToolChoice = "auto"
	}

	// Request diagnostics for 500 errors

	// Retry logic for 500 errors
	var resp openai.ChatCompletionResponse
	var err error
	maxRetries := 7

	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err = h.client.CreateChatCompletion(ctx, req)
		if err != nil {
			var apiErr *openai.APIError
			if errors.As(err, &apiErr) {
				// Retry on 500 errors
				if apiErr.HTTPStatusCode == 500 && attempt < maxRetries {
					waitTime := time.Duration(1<<attempt) * time.Second // 1s, 2s, 4s, 8s, 16s, 32s
					// Retry after 500 error
					time.Sleep(waitTime)
					continue
				}

				code := "unknown"
				if apiErr.Code != nil {
					code = fmt.Sprintf("%v", apiErr.Code)
				}
				param := "none"
				if apiErr.Param != nil {
					param = *apiErr.Param
				}
				return nil, fmt.Errorf("%s completion error: %s (code: %s, type: %s, param: %s, http_status: %d)",
					h.providerName, apiErr.Message, code, apiErr.Type, param, apiErr.HTTPStatusCode)
			}
			return nil, fmt.Errorf("%s completion error: %w", h.providerName, err)
		}
		break // Success
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("%s returned no choices", h.providerName)
	}

	choice := resp.Choices[0].Message

	// Convert OpenAI tool calls to our format
	var toolCalls []entity.ToolCall
	for _, toolCall := range choice.ToolCalls {
		entityToolCall := entity.ToolCall{
			ID:   toolCall.ID,
			Type: string(toolCall.Type),
			Function: entity.FunctionCall{
				Name:      toolCall.Function.Name,
				Arguments: toolCall.Function.Arguments,
			},
			Name:      toolCall.Function.Name,
			Arguments: toolCall.Function.Arguments,
		}

		// Parse arguments into Args map
		if toolCall.Function.Arguments != "" {
			var args map[string]interface{}
			if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err == nil {
				entityToolCall.Args = args
			}
		}

		toolCalls = append(toolCalls, entityToolCall)
	}

	return &entity.AIResponse{
		Content:   choice.Content,
		ToolCalls: toolCalls,
	}, nil
}

func (h *OpenAICompatibleHandler) validateContext(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("context cannot be nil")
	}
	select {
	case <-ctx.Done():
		return fmt.Errorf("context canceled: %w", ctx.Err())
	default:
		return nil
	}
}

// convertEntityMessageToOpenAI converts entity.Message to openai.ChatCompletionMessage
func (h *OpenAICompatibleHandler) convertEntityMessageToOpenAI(msg entity.Message) *openai.ChatCompletionMessage {
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
			var argsJSON string
			if tc.Arguments != "" {
				argsJSON = tc.Arguments
			} else if tc.Args != nil {
				argsBytes, _ := json.Marshal(tc.Args)
				argsJSON = string(argsBytes)
			}

			toolCalls = append(toolCalls, openai.ToolCall{
				ID:   tc.ID,
				Type: openai.ToolTypeFunction,
				Function: openai.FunctionCall{
					Name:      tc.Function.Name,
					Arguments: argsJSON,
				},
			})
		}
		return &openai.ChatCompletionMessage{
			Role:      role,
			Content:   msg.Content,
			ToolCalls: toolCalls,
		}
	} else if msg.ToolCallID != "" {
		// Tool results - different handling for OpenRouter vs other providers
		return &openai.ChatCompletionMessage{
			Role:       openai.ChatMessageRoleTool,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		}
	} else {
		return &openai.ChatCompletionMessage{
			Role:    role,
			Content: msg.Content,
		}
	}
}

func (h *OpenAICompatibleHandler) GetModel() ModelInfo {
	modelID := h.config.Model
	if modelID == "" {
		modelID = "gpt-4"
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
		SupportsImages:       h.capabilities.Images,
		SupportsTools:        h.capabilities.Tools,
		SupportsSystemPrompt: h.capabilities.SystemPrompts,
		Description:          fmt.Sprintf("%s model", h.providerName),
	}
}
