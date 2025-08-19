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
	// Use a channel to handle stdin reading non-blocking
	inputChan := make(chan string)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)

		for scanner.Scan() {
			input := strings.TrimSpace(scanner.Text())
			inputChan <- input
		}
		close(inputChan)
	}()

	for input := range inputChan {
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
			fmt.Fprintf(os.Stderr, "Task processing error: %v\n", err)
		}
	}

	return nil
}
