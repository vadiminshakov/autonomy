package ai

import (
	"context"
	"fmt"
	"strings"

	"github.com/vadiminshakov/autonomy/core/config"
	"github.com/vadiminshakov/autonomy/core/entity"
)

type AIClient interface {
	GenerateCode(ctx context.Context, promptData entity.PromptData) (*entity.AIResponse, error)
}

func ProvideAiClient(cfg config.Config) (AIClient, error) {
	provider := strings.ToLower(cfg.Provider)

	switch provider {
	case "anthropic":
		return NewAnthropicProvider(cfg)

	case "openai":
		return NewOpenAICompatibleProvider(cfg, "OpenAI"), nil

	case "openrouter":
		if cfg.BaseURL == "" {
			cfg.BaseURL = "https://openrouter.ai/api/v1"
		}
		return NewOpenAICompatibleProvider(cfg, "OpenRouter"), nil

	case "groq":
		return NewOpenAICompatibleProvider(cfg, "Groq"), nil

	case "deepseek":
		return NewOpenAICompatibleProvider(cfg, "DeepSeek"), nil

	case "ollama":
		if cfg.BaseURL == "" {
			cfg.BaseURL = "http://localhost:11434/v1"
		}
		return NewOpenAICompatibleProvider(cfg, "Ollama"), nil

	case "local":
		if cfg.BaseURL == "" {
			cfg.BaseURL = "http://localhost:11434/v1"
		}
		return NewOpenAICompatibleProvider(cfg, "Local"), nil

	default:
		return nil, fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}
}
