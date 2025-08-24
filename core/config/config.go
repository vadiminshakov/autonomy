package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/manifoldco/promptui"

	"github.com/vadiminshakov/autonomy/core/entity"
)

const (
	defaultOpenAIURL     = "https://api.openai.com/v1"
	DefaultAnthropicURL  = "https://api.anthropic.com"
	defaultOpenRouterURL = "https://openrouter.ai/api/v1"

	configDirName  = ".autonomy"
	configFileName = "config.json"
)

type Config struct {
	APIKey       string                  `json:"api_key"`
	BaseURL      string                  `json:"base_url"`
	Model        string                  `json:"model"`
	Provider     string                  `json:"provider"`
	MaxTokens    int                     `json:"max_tokens,omitempty"`
	Temperature  float64                 `json:"temperature,omitempty"`
	UseAuthToken bool                    `json:"use_auth_token,omitempty"`
	Tools        []entity.ToolDefinition `json:"tools,omitempty"`
}

func configFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to detect home directory: %w", err)
	}
	return filepath.Join(home, configDirName, configFileName), nil
}

func LoadConfigFile() (Config, error) {
	var cfg Config

	path, err := configFilePath()
	if err != nil {
		return cfg, err
	}

	f, err := os.Open(path)
	if err != nil {
		return cfg, err
	}
	defer f.Close()

	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func saveConfigFile(cfg Config) error {
	path, err := configFilePath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(cfg)
}

//nolint:gocyclo
func InteractiveSetup() (Config, error) {
	configTypes := []string{"cloud", "local"}
	typeSel := promptui.Select{
		Label: "Select configuration type",
		Items: configTypes,
	}
	_, typeChoice, err := typeSel.Run()
	if err != nil {
		return Config{}, err
	}

	var cfg Config

	switch typeChoice {
	case "cloud":
		providers := []string{"openai", "anthropic", "openrouter"}
		provSel := promptui.Select{
			Label: "Select cloud provider",
			Items: providers,
		}
		_, provChoice, err := provSel.Run()
		if err != nil {
			return cfg, err
		}
		cfg.Provider = provChoice

		key, err := readInput("Enter API key: ")
		if err != nil {
			return cfg, err
		}
		cfg.APIKey = strings.TrimSpace(key)

		if cfg.Provider == "openrouter" {
			cfg.BaseURL = defaultOpenRouterURL
		} else {
			defaultURL := defaultOpenAIURL
			if cfg.Provider == "anthropic" {
				defaultURL = DefaultAnthropicURL
			}

			urlPrompt := promptui.Prompt{
				Label:   fmt.Sprintf("Enter Base URL (default %s)", defaultURL),
				Default: defaultURL,
			}
			bu, err := urlPrompt.Run()
			if err != nil {
				return cfg, err
			}
			cfg.BaseURL = strings.TrimSpace(bu)

			if cfg.BaseURL == "" {
				cfg.BaseURL = defaultURL
			}
		}

		var modelOptions []string
		switch cfg.Provider {
		case "openai":
			modelOptions = []string{"o4", "o3", "gpt-4.1", "gpt-4o"}
		case "openrouter":
			modelOptions = []string{
				"google/gemini-2.5-pro",
				"x-ai/grok-4",
				"moonshotai/kimi-k2",
				"qwen/qwen3-coder",
				"deepseek/deepseek-chat-v3-0324",
			}
		case "anthropic":
			modelOptions = []string{"claude-opus-4-1-20250805", "claude-sonnet-4-20250514", "claude-3-7-sonnet-20250219"}
		}

		modelOptions = append(modelOptions, "<enter custom model>")

		modelSel := promptui.Select{
			Label: "Select model (use ↑↓ and Enter)",
			Items: modelOptions,
		}
		_, modelChoice, err := modelSel.Run()
		if err != nil {
			return cfg, err
		}

		if modelChoice == "<enter custom model>" {
			customPrompt := promptui.Prompt{
				Label: "Enter model name",
			}
			customModel, err := customPrompt.Run()
			if err != nil {
				return cfg, err
			}
			cfg.Model = strings.TrimSpace(customModel)
		} else {
			cfg.Model = modelChoice
		}

	case "local":
		cfg.Provider = "openai"

		urlPrompt := promptui.Prompt{
			Label: "Enter Base URL (e.g., http://localhost:11434/v1)",
		}
		bu, err := urlPrompt.Run()
		if err != nil {
			return cfg, err
		}
		cfg.BaseURL = strings.TrimSpace(bu)

		key, err := readInput("Enter API key (leave blank if not required): ")
		if err != nil {
			return cfg, err
		}
		cfg.APIKey = strings.TrimSpace(key)

		modelPrompt := promptui.Prompt{
			Label: "Enter model name (optional)",
		}
		mn, err := modelPrompt.Run()
		if err != nil {
			return cfg, err
		}
		mn = strings.TrimSpace(mn)
		if mn != "" {
			cfg.Model = mn
		}
	}

	if err := cfg.Validate(); err != nil {
		return cfg, err
	}

	if err := saveConfigFile(cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func readInput(prompt string) (string, error) {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(input), nil
}

func (c *Config) HasValidCredentials() bool {
	return c.APIKey != "" || c.BaseURL != ""
}

func (c *Config) IsLocalModel() bool {
	return c.BaseURL != "" && (c.APIKey == "" || c.APIKey == "EMPTY")
}

func (c *Config) Validate() error {
	if c.Provider == "openai" && c.IsLocalModel() && c.APIKey == "" {
		c.APIKey = "local-api-key"
	}

	if !c.HasValidCredentials() {
		return fmt.Errorf("either APIKey or BaseURL must be provided for provider %s", c.Provider)
	}

	if c.APIKey != "" && c.APIKey != "local-api-key" && c.BaseURL == "" {
		return fmt.Errorf("BaseURL is required when APIKey is provided for provider %s", c.Provider)
	}

	return nil
}
