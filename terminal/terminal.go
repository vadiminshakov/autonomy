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

func newAIClient(cfg config.Config) (ai.AIClient, error) {
	return ai.ProvideAiClient(cfg)
}

func RunTerminal(client ai.AIClient) error {
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

func RunHeadless(client ai.AIClient) error {
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Fprintf(os.Stdout, "Autonomy agent is ready\n")
	os.Stdout.Sync()

	for scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())

		if input == "" {
			continue
		}

		switch input {
		case "exit", "quit":
			return nil
		case "status":
			continue
		case "ping":
			continue
		}

		t := task.NewTask(client)
		t.SetOriginalTask(input)

		t.AddUserMessage(input)

		err := t.ProcessTask()
		t.Close()

		if err != nil {
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading input: %v", err)
	}

	return nil
}
