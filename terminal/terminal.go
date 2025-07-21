package terminal

import (
	"fmt"

	"github.com/vadiminshakov/autonomy/core/ai"
	"github.com/vadiminshakov/autonomy/core/config"
	"github.com/vadiminshakov/autonomy/core/task"
	"github.com/vadiminshakov/autonomy/ui"
)

// newAIClient creates an AI client based on the provided configuration
func newAIClient(cfg config.Config) (task.AIClient, error) {
	switch cfg.Provider {
	case "openai":
		return ai.NewOpenai(cfg)
	case "anthropic":
		return ai.NewAnthropic(cfg)
	case "openrouter":
		return ai.NewOpenai(cfg)
	default:
		return nil, fmt.Errorf("unknown provider %s in config", cfg.Provider)
	}
}

func RunTerminal(client task.AIClient) error {
	repl := ui.NewREPL()
	defer repl.Close()
	repl.ShowWelcome()

	for {
		input, shouldExit, isReconfig := repl.ReadInput()
		if shouldExit {
			break
		}

		if isReconfig {
			cfg, err := config.LoadConfigFile()
			if err != nil {
				ui.ShowError(fmt.Errorf("failed to load new configuration: %w", err))
				continue
			}
			newClient, err := newAIClient(cfg)
			if err != nil {
				ui.ShowError(fmt.Errorf("failed to create ai client: %w", err))
				continue
			}
			client = newClient
			continue
		}

		if input == "" {
			continue
		}

		ui.ShowTaskStart(input)

		t := task.NewTask(client)
		defer t.Close()

		t.AddUserMessage(input)

		err := t.ProcessTask()
		if err != nil {
			ui.ShowError(err)
		} else {
			ui.ShowTaskComplete()
		}
	}

	return nil
}
