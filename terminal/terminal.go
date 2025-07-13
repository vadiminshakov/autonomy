package terminal

import (
	"autonomy/core/index"
	"autonomy/core/task"
	"autonomy/ui"
	"fmt"
)

func RunTerminal(client task.AIClient) error {
	indexManager := index.GetIndexManager()
	go func() {
		if err := indexManager.Initialize(); err != nil {
			fmt.Printf("Warning: failed to initialize index: %v\n", err)
		}
	}()

	repl := ui.NewREPL()
	defer repl.Close()
	repl.ShowWelcome()

	for {
		input, shouldExit := repl.ReadInput()
		if shouldExit {
			break
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
