package ai

import (
	"context"
	"fmt"

	"github.com/vadiminshakov/autonomy/core/entity"
)

type BaseProvider struct {
	providerType ProviderType
	capabilities ProviderCapabilities
	baseURL      string
	apiKey       string
	defaultModel string
}

func NewBaseProvider(providerType ProviderType, capabilities ProviderCapabilities) *BaseProvider {
	return &BaseProvider{
		providerType: providerType,
		capabilities: capabilities,
	}
}

func NewRouterProvider(providerType ProviderType, capabilities ProviderCapabilities, baseURL, apiKey, defaultModel string) *BaseProvider {
	return &BaseProvider{
		providerType: providerType,
		capabilities: capabilities,
		baseURL:      baseURL,
		apiKey:       apiKey,
		defaultModel: defaultModel,
	}
}

func (b *BaseProvider) GetProviderType() ProviderType {
	return b.providerType
}

func (b *BaseProvider) GetCapabilities() ProviderCapabilities {
	return b.capabilities
}

func (b *BaseProvider) GetModel() ModelInfo {
	return ModelInfo{
		ID:                   "unknown",
		MaxTokens:            4096,
		Temperature:          0.0,
		ContextWindow:        4096,
		SupportsImages:       b.capabilities.Images,
		SupportsTools:        b.capabilities.Tools,
		SupportsSystemPrompt: b.capabilities.SystemPrompts,
		Description:          fmt.Sprintf("%s model", b.providerType),
	}
}

func (b *BaseProvider) CountTokens(ctx context.Context, content []entity.Message) (int, error) {
	totalChars := 0
	for _, msg := range content {
		totalChars += len(msg.Content)
		for _, toolCall := range msg.ToolCalls {
			totalChars += len(toolCall.Function.Name)
			totalChars += len(toolCall.Arguments)
		}
	}
	return totalChars / 4, nil
}

func (b *BaseProvider) validateContext(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("context cannot be nil")
	}
	select {
	case <-ctx.Done():
		return fmt.Errorf("context cancelled: %w", ctx.Err())
	default:
		return nil
	}
}

func (b *BaseProvider) GetBaseURL() string {
	return b.baseURL
}

func (b *BaseProvider) GetAPIKey() string {
	return b.apiKey
}
