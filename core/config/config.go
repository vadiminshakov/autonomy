package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/manifoldco/promptui"
)

const (
	defaultOpenAIURL     = "https://api.openai.com/v1"
	DefaultAnthropicURL  = "https://api.anthropic.com"
	defaultOpenRouterURL = "https://openrouter.ai/api/v1"

	configDirName  = ".autonomy"
	configFileName = "config.json"
)

type Config struct {
	APIKey   string `json:"api_key"`
	BaseURL  string `json:"base_url"`
	Model    string `json:"model"`
	Provider string `json:"provider"`
}

// configFilePath builds the path to ~/.autonomy/config.json.
func configFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to detect home directory: %w", err)
	}
	return filepath.Join(home, configDirName, configFileName), nil
}

// LoadConfigFile reads configuration from ~/.autonomy/config.json.
// Returns an error if the file does not exist or cannot be parsed.
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

// saveConfigFile writes the provided configuration to ~/.autonomy/config.json.
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

// InteractiveSetup launches a CLI wizard to collect configuration from the user
// and saves the result to ~/.autonomy/config.json.
//
//nolint:gocyclo
func InteractiveSetup() (Config, error) {
	fmt.Println("🔧 Initial configuration (Autonomy)")

	// Step 1: choose config type (cloud vs local)
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
		// Provider selection
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

		// API key (masked)
		keyPrompt := promptui.Prompt{
			Label: "Enter API key",
		}
		key, err := keyPrompt.Run()
		if err != nil {
			return cfg, err
		}
		cfg.APIKey = strings.TrimSpace(key)

		// base URL handling
		if cfg.Provider == "openrouter" {
			cfg.BaseURL = defaultOpenRouterURL
			fmt.Printf("Base URL set to %s\n", defaultOpenRouterURL)
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

			// ensure default URL is set if user left the input blank
			if cfg.BaseURL == "" {
				cfg.BaseURL = defaultURL
			}
		}

		// model selection
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
			modelOptions = []string{"claude-4-opus", "claude-4-sonnet-20250514", "claude-3-7-sonnet"}
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
		// local model / OpenAI-compatible API
		cfg.Provider = "openai"

		// base URL
		urlPrompt := promptui.Prompt{
			Label: "Enter Base URL (e.g., http://localhost:11434/v1)",
		}
		bu, err := urlPrompt.Run()
		if err != nil {
			return cfg, err
		}
		cfg.BaseURL = strings.TrimSpace(bu)

		// API key (optional)
		keyPrompt := promptui.Prompt{
			Label: "Enter API key (leave blank if not required)",
		}
		key, err := keyPrompt.Run()
		if err != nil {
			return cfg, err
		}
		cfg.APIKey = strings.TrimSpace(key)

		// model name (optional)
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

	fmt.Println("Configuration saved to ~/.autonomy/config.json ✅")

	return cfg, nil
}

// HasValidCredentials checks if the config has valid credentials
func (c *Config) HasValidCredentials() bool {
	return c.APIKey != "" || c.BaseURL != ""
}

// IsLocalModel checks if the config is for a local model
func (c *Config) IsLocalModel() bool {
	return c.BaseURL != "" && (c.APIKey == "" || c.APIKey == "EMPTY")
}

// Validate validates the configuration and applies necessary fixes
func (c *Config) Validate() error {
	// auto-set a dummy API key only for truly local OpenAI-compatible setups
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
