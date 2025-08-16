package ai

type ModelInfo struct {
	ID                   string  `json:"id"`
	MaxTokens            int     `json:"max_tokens"`
	Temperature          float64 `json:"temperature"`
	ContextWindow        int     `json:"context_window"`
	SupportsImages       bool    `json:"supports_images"`
	SupportsTools        bool    `json:"supports_tools"`
	SupportsSystemPrompt bool    `json:"supports_system_prompt"`
	Description          string  `json:"description"`
}

type ProviderType string

const (
	ProviderTypeAnthropic  ProviderType = "anthropic"
	ProviderTypeOpenAI     ProviderType = "openai"
	ProviderTypeOpenRouter ProviderType = "openrouter"
	ProviderTypeOllama     ProviderType = "ollama"
	ProviderTypeGroq       ProviderType = "groq"
	ProviderTypeDeepSeek   ProviderType = "deepseek"
	ProviderTypeLocal      ProviderType = "local"
)

type ProviderCapabilities struct {
	Tools         bool `json:"tools"`
	Images        bool `json:"images"`
	SystemPrompts bool `json:"system_prompts"`
}
