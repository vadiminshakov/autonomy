package terminal

import (
	"fmt"

	"github.com/vadiminshakov/autonomy/core/index"
	"github.com/vadiminshakov/autonomy/core/task"
	"github.com/vadiminshakov/autonomy/ui"
)

func RunTerminal(client task.AIClient) error {
	indexManager := index.GetIndexManager()
	go func() {
		if err := indexManager.Initialize(); err != nil {
			ui.ShowIndexWarning(fmt.Sprintf("Failed to initialize index: %v", err))
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
