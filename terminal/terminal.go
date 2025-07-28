package terminal

import (
	"bufio"
	"fmt"
	"os"
	"strings"

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
	case "local":
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
		t.SetOriginalTask(input)
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

// RunHeadless runs the terminal in headless mode for VS Code extension integration
func RunHeadless(client task.AIClient) error {
	scanner := bufio.NewScanner(os.Stdin)

	for scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())

		if input == "" {
			continue
		}

		// Handle special commands
		switch input {
		case "exit", "quit":
			fmt.Printf("Shutting down autonomy agent\n")
			return nil
		case "status":
			fmt.Printf("Agent is running and ready\n")
			continue
		}

		fmt.Printf("Processing task: %s\n", input)

		t := task.NewTask(client)
		t.SetOriginalTask(input)
		defer t.Close()

		t.AddUserMessage(input)

		err := t.ProcessTask()
		if err != nil {
			fmt.Printf("Task failed: %v\n", err)
		} else {
			fmt.Printf("Task completed successfully\n")
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading input: %v", err)
	}

	return nil
}
