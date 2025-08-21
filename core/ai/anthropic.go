package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/vadiminshakov/autonomy/core/config"
	"github.com/vadiminshakov/autonomy/core/entity"
)

type AnthropicHandler struct {
	providerType ProviderType
	capabilities ProviderCapabilities
	client       *http.Client
	config       config.Config
	baseURL      string
	apiKey       string
	modelID      string
}

type AnthropicContent struct {
	Type      string                 `json:"type"`
	Text      string                 `json:"text,omitempty"`
	ID        string                 `json:"id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Input     map[string]interface{} `json:"input,omitempty"`
	ToolUseID string                 `json:"tool_use_id,omitempty"`
	Source    *struct {
		Type      string `json:"type"`
		MediaType string `json:"media_type"`
		Data      string `json:"data"`
	} `json:"source,omitempty"`
}

type AnthropicMessage struct {
	Role    string             `json:"role"`
	Content []AnthropicContent `json:"content"`
}

type AnthropicTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema struct {
		Type       string                 `json:"type"`
		Properties map[string]interface{} `json:"properties,omitempty"`
		Required   []string               `json:"required,omitempty"`
	} `json:"input_schema"`
}

type AnthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	Messages    []AnthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	Tools       []AnthropicTool    `json:"tools,omitempty"`
	Temperature float64            `json:"temperature,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
}

type AnthropicToolUse struct {
	Type  string                 `json:"type"`
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

type AnthropicResponse struct {
	ID      string             `json:"id"`
	Type    string             `json:"type"`
	Role    string             `json:"role"`
	Content []AnthropicContent `json:"content"`
	Model   string             `json:"model"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	StopReason string `json:"stop_reason"`
}

type AnthropicError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type AnthropicErrorResponse struct {
	Type  string         `json:"type"`
	Error AnthropicError `json:"error"`
}

type AnthropicStreamEvent struct {
	Type         string                `json:"type"`
	Index        int                   `json:"index,omitempty"`
	Delta        *AnthropicStreamDelta `json:"delta,omitempty"`
	Message      *AnthropicResponse    `json:"message,omitempty"`
	ContentBlock *AnthropicContent     `json:"content_block,omitempty"`
}

type AnthropicStreamDelta struct {
	Type       string `json:"type,omitempty"`
	Text       string `json:"text,omitempty"`
	StopReason string `json:"stop_reason,omitempty"`
}

func NewAnthropicProvider(cfg config.Config) (*AnthropicHandler, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}

	capabilities := ProviderCapabilities{
		Tools:         true,
		Images:        true,
		SystemPrompts: true,
	}

	modelID := cfg.Model
	if modelID == "" {
		modelID = "claude-3-5-sonnet-20241022"
	}

	return &AnthropicHandler{
		providerType: ProviderTypeAnthropic,
		capabilities: capabilities,
		client:       &http.Client{Timeout: 120 * time.Second}, // Longer timeout for complex requests
		config:       cfg,
		baseURL:      baseURL,
		apiKey:       cfg.APIKey,
		modelID:      modelID,
	}, nil
}

func (h *AnthropicHandler) GenerateCode(ctx context.Context, promptData entity.PromptData) (*entity.AIResponse, error) {
	if err := h.validateContext(ctx); err != nil {
		return nil, err
	}

	// Convert tools to Anthropic format
	anthropicTools := h.convertTools(promptData.Tools)

	// Convert messages to Anthropic format
	var anthropicMessages []AnthropicMessage
	for _, msg := range promptData.Messages {
		anthropicMsg := h.convertMessage(msg)
		if anthropicMsg != nil {
			anthropicMessages = append(anthropicMessages, *anthropicMsg)
		}
	}

	// Build request
	temperature := 0.0
	if h.config.Temperature >= 0 {
		temperature = h.config.Temperature
	}

	reqData := AnthropicRequest{
		Model:       h.modelID,
		MaxTokens:   h.getMaxTokens(),
		Messages:    anthropicMessages,
		System:      promptData.SystemPrompt,
		Tools:       anthropicTools,
		Temperature: temperature,
		Stream:      false,
	}

	return h.sendRequest(ctx, reqData)
}

// GenerateCodeStream generates code with streaming response
func (h *AnthropicHandler) GenerateCodeStream(ctx context.Context, promptData entity.PromptData) (<-chan string, <-chan error) {
	textChan := make(chan string, 100)
	errorChan := make(chan error, 1)

	go func() {
		defer close(textChan)
		defer close(errorChan)

		// Convert tools and messages same as non-streaming version
		anthropicTools := h.convertTools(promptData.Tools)

		var anthropicMessages []AnthropicMessage
		for _, msg := range promptData.Messages {
			anthropicMsg := h.convertMessage(msg)
			if anthropicMsg != nil {
				anthropicMessages = append(anthropicMessages, *anthropicMsg)
			}
		}

		temperature := 0.0
		if h.config.Temperature >= 0 {
			temperature = h.config.Temperature
		}

		reqData := AnthropicRequest{
			Model:       h.modelID,
			MaxTokens:   h.getMaxTokens(),
			Messages:    anthropicMessages,
			System:      promptData.SystemPrompt,
			Tools:       anthropicTools,
			Temperature: temperature,
			Stream:      true,
		}

		if err := h.sendStreamRequest(ctx, reqData, textChan); err != nil {
			errorChan <- err
		}
	}()

	return textChan, errorChan
}

//nolint:gocyclo
func (h *AnthropicHandler) sendStreamRequest(ctx context.Context, reqData AnthropicRequest, textChan chan<- string) error {
	jsonData, err := json.Marshal(reqData)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", h.baseURL+"/v1/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", h.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("anthropic api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		var errorResp AnthropicErrorResponse
		if json.Unmarshal(body, &errorResp) == nil {
			return fmt.Errorf("anthropic api error (%d): %s - %s",
				resp.StatusCode, errorResp.Error.Type, errorResp.Error.Message)
		}
		return fmt.Errorf("anthropic api error (%d): %s", resp.StatusCode, string(body))
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")

			if data == "[DONE]" {
				break
			}

			var event AnthropicStreamEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue // Skip malformed events
			}

			// Handle different event types
			switch event.Type {
			case "content_block_delta":
				if event.Delta != nil && event.Delta.Text != "" {
					textChan <- event.Delta.Text
				}
			case "message_delta":
				if event.Delta != nil && event.Delta.StopReason != "" {
					// Message completed
					return nil
				}
			}
		}
	}

	return scanner.Err()
}

func (h *AnthropicHandler) convertMessage(msg entity.Message) *AnthropicMessage {
	switch msg.Role {
	case "user", "assistant":
		anthropicMsg := AnthropicMessage{
			Role: msg.Role,
		}

		// Handle text content
		if msg.Content != "" {
			anthropicMsg.Content = append(anthropicMsg.Content, AnthropicContent{
				Type: "text",
				Text: msg.Content,
			})
		}

		// Handle tool calls (for assistant messages)
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			for _, toolCall := range msg.ToolCalls {
				// Parse arguments
				var input map[string]interface{}
				if toolCall.Arguments != "" {
					if err := json.Unmarshal([]byte(toolCall.Arguments), &input); err != nil {
						// Skip malformed tool call arguments
						continue
					}
				} else if toolCall.Args != nil {
					input = toolCall.Args
				}

				// Create tool use content using AnthropicContent with proper structure
				toolContent := AnthropicContent{
					Type:  "tool_use",
					ID:    toolCall.ID,
					Name:  toolCall.Function.Name,
					Input: input,
				}

				anthropicMsg.Content = append(anthropicMsg.Content, toolContent)
			}
		}

		return &anthropicMsg

	case "tool":
		// Tool results are sent as user messages in Anthropic API
		return &AnthropicMessage{
			Role: "user",
			Content: []AnthropicContent{
				{
					Type:      "tool_result",
					Text:      msg.Content,
					ToolUseID: msg.ToolCallID,
				},
			},
		}

	default:
		return nil
	}
}

func (h *AnthropicHandler) sendRequest(ctx context.Context, reqData AnthropicRequest) (*entity.AIResponse, error) {
	// Validate request before sending
	if err := h.validateRequest(reqData); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	jsonData, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", h.baseURL+"/v1/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", h.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("User-Agent", "Autonomy/1.0")

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, h.wrapHTTPError(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, h.parseError(resp.StatusCode, body)
	}

	var anthropicResp AnthropicResponse
	if err := json.Unmarshal(body, &anthropicResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return h.convertResponse(anthropicResp)
}

//nolint:gocyclo
func (h *AnthropicHandler) parseError(statusCode int, body []byte) error {
	var errorResp AnthropicErrorResponse
	if json.Unmarshal(body, &errorResp) == nil {
		switch errorResp.Error.Type {
		case "authentication_error":
			return fmt.Errorf("authentication failed - check your API key: %s", errorResp.Error.Message)
		case "permission_error":
			return fmt.Errorf("permission denied - check API key permissions: %s", errorResp.Error.Message)
		case "not_found_error":
			return fmt.Errorf("resource not found - check model name: %s", errorResp.Error.Message)
		case "rate_limit_error":
			return fmt.Errorf("rate limit exceeded - please try again later: %s", errorResp.Error.Message)
		case "api_error":
			return fmt.Errorf("anthropic api error: %s", errorResp.Error.Message)
		case "overloaded_error":
			return fmt.Errorf("anthropic servers overloaded - please try again: %s", errorResp.Error.Message)
		default:
			return fmt.Errorf("anthropic api error (%d): %s - %s",
				statusCode, errorResp.Error.Type, errorResp.Error.Message)
		}
	}

	// Fallback for unparseable errors
	switch statusCode {
	case 400:
		return fmt.Errorf("bad request (400): %s", string(body))
	case 401:
		return fmt.Errorf("unauthorized (401) - check your API key: %s", string(body))
	case 403:
		return fmt.Errorf("forbidden (403) - insufficient permissions: %s", string(body))
	case 404:
		return fmt.Errorf("not found (404) - check endpoint/model: %s", string(body))
	case 429:
		return fmt.Errorf("rate limited (429) - too many requests: %s", string(body))
	case 500:
		return fmt.Errorf("internal server error (500) - anthropic issue: %s", string(body))
	case 502:
		return fmt.Errorf("bad gateway (502) - anthropic service issue: %s", string(body))
	case 503:
		return fmt.Errorf("service unavailable (503) - anthropic overloaded: %s", string(body))
	default:
		return fmt.Errorf("anthropic api error (%d): %s", statusCode, string(body))
	}
}

//nolint:unparam
func (h *AnthropicHandler) convertResponse(resp AnthropicResponse) (*entity.AIResponse, error) {
	var aiResponse entity.AIResponse
	var textParts []string

	for _, content := range resp.Content {
		switch content.Type {
		case "text":
			textParts = append(textParts, content.Text)
		case "tool_use":
			// Convert tool use to ToolCall
			argsBytes, _ := json.Marshal(content.Input)

			toolCall := entity.NewToolCall(
				content.ID,
				"function",
				entity.FunctionCall{
					Name:      content.Name,
					Arguments: string(argsBytes),
				},
			)

			aiResponse.ToolCalls = append(aiResponse.ToolCalls, toolCall)
		}
	}

	if len(textParts) > 0 {
		aiResponse.Content = strings.Join(textParts, "\n")
	}

	return &aiResponse, nil
}

func (h *AnthropicHandler) CompletePrompt(ctx context.Context, prompt string) (string, error) {
	if err := h.validateContext(ctx); err != nil {
		return "", err
	}

	if strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("prompt cannot be empty")
	}

	// Simple text completion with proper temperature
	temperature := 0.0
	if h.config.Temperature >= 0 {
		temperature = h.config.Temperature
	}

	reqData := AnthropicRequest{
		Model:       h.modelID,
		MaxTokens:   h.getMaxTokens(),
		Temperature: temperature,
		Messages: []AnthropicMessage{
			{
				Role: "user",
				Content: []AnthropicContent{
					{Type: "text", Text: prompt},
				},
			},
		},
		Stream: false,
	}

	response, err := h.sendRequest(ctx, reqData)
	if err != nil {
		return "", err
	}

	return response.Content, nil
}

func (h *AnthropicHandler) GetModel() ModelInfo {
	maxTokens := h.getMaxTokens()
	if h.config.MaxTokens > 0 {
		maxTokens = h.config.MaxTokens
	}

	temperature := 0.0
	if h.config.Temperature >= 0 {
		temperature = h.config.Temperature
	}

	return ModelInfo{
		ID:                   h.modelID,
		MaxTokens:            maxTokens,
		Temperature:          temperature,
		ContextWindow:        200000, // Claude 3.5 Sonnet context window
		SupportsImages:       true,   // Now supports images
		SupportsTools:        true,   // Now supports tools
		SupportsSystemPrompt: true,
		Description:          "Anthropic Claude model with full capabilities",
	}
}

func (h *AnthropicHandler) getMaxTokens() int {
	if h.config.MaxTokens > 0 {
		return h.config.MaxTokens
	}
	return 16384
}

// CountTokens provides a rough token estimate
func (h *AnthropicHandler) CountTokens(ctx context.Context, content []entity.Message) (int, error) {
	totalChars := 0
	for _, msg := range content {
		totalChars += len(msg.Content)
		for _, toolCall := range msg.ToolCalls {
			totalChars += len(toolCall.Function.Name)
			totalChars += len(toolCall.Arguments)
		}
	}
	// Anthropic uses approximately 4 characters per token
	return totalChars / 4, nil
}

// AddImageSupport adds an image to a message (base64 encoded)
func (h *AnthropicHandler) AddImageToMessage(content []AnthropicContent, imageData, mimeType string) []AnthropicContent {
	imageContent := AnthropicContent{
		Type: "image",
		Source: &struct {
			Type      string `json:"type"`
			MediaType string `json:"media_type"`
			Data      string `json:"data"`
		}{
			Type:      "base64",
			MediaType: mimeType,
			Data:      imageData,
		},
	}
	return append(content, imageContent)
}

// validateRequest validates the request before sending to Anthropic API
func (h *AnthropicHandler) validateRequest(req AnthropicRequest) error {
	if req.Model == "" {
		return fmt.Errorf("model is required")
	}
	if req.MaxTokens <= 0 {
		return fmt.Errorf("max_tokens must be positive")
	}
	if len(req.Messages) == 0 {
		return fmt.Errorf("messages array cannot be empty")
	}

	// Validate message structure
	for i, msg := range req.Messages {
		if msg.Role == "" {
			return fmt.Errorf("message %d: role is required", i)
		}
		if len(msg.Content) == 0 {
			return fmt.Errorf("message %d: content cannot be empty", i)
		}

		// Validate content blocks
		for j, content := range msg.Content {
			if content.Type == "" {
				return fmt.Errorf("message %d, content %d: type is required", i, j)
			}
		}
	}

	return nil
}

// wrapHTTPError wraps HTTP errors with more context
func (h *AnthropicHandler) wrapHTTPError(err error) error {
	if strings.Contains(err.Error(), "timeout") {
		return fmt.Errorf("request timeout - Anthropic API may be slow: %w", err)
	}
	if strings.Contains(err.Error(), "connection refused") {
		return fmt.Errorf("connection refused - check if Anthropic API is accessible: %w", err)
	}
	if strings.Contains(err.Error(), "no such host") {
		return fmt.Errorf("DNS resolution failed - check internet connection: %w", err)
	}
	return fmt.Errorf("anthropic api request failed: %w", err)
}

// convertTool converts entity tool definition to Anthropic tool format
func (h *AnthropicHandler) convertTool(tool entity.ToolDefinition) AnthropicTool {
	anthropicTool := AnthropicTool{
		Name:        tool.Name,
		Description: tool.Description,
	}

	if tool.InputSchema != nil {
		anthropicTool.InputSchema.Type = "object"
		if properties, ok := tool.InputSchema["properties"].(map[string]interface{}); ok {
			anthropicTool.InputSchema.Properties = properties
		}
		if required, ok := tool.InputSchema["required"].([]interface{}); ok {
			for _, req := range required {
				if reqStr, ok := req.(string); ok {
					anthropicTool.InputSchema.Required = append(anthropicTool.InputSchema.Required, reqStr)
				}
			}
		}
	}

	return anthropicTool
}

// convertTools converts entity tool definitions to Anthropic tools format
func (h *AnthropicHandler) convertTools(tools []entity.ToolDefinition) []AnthropicTool {
	var anthropicTools []AnthropicTool
	for _, tool := range tools {
		anthropicTools = append(anthropicTools, h.convertTool(tool))
	}

	return anthropicTools
}

func (h *AnthropicHandler) validateContext(ctx context.Context) error {
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
