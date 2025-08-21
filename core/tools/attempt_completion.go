package tools

import (
	"errors"
	"fmt"
)

func init() {
	Register("attempt_completion", AttemptCompletion)
}

// AttemptCompletion marks the task as completed and returns a final message.
func AttemptCompletion(args map[string]interface{}) (string, error) {
	result, _ := args["result"].(string)

	// do not allow completion if there are recorded errors
	state := getTaskState()
	if len(state.Errors) > 0 {
		// show last 3 errors for context
		errCount := len(state.Errors)
		showErrors := 3
		if errCount < showErrors {
			showErrors = errCount
		}

		errMsg := fmt.Sprintf("cannot complete task: %d unresolved errors\n", errCount)
		errMsg += "Recent errors:\n"
		for i := errCount - showErrors; i < errCount; i++ {
			errMsg += fmt.Sprintf("  %d. %s\n", i+1, state.Errors[i])
		}
		errMsg += "\nHint: Fix the errors or use 'reset_task_state' to clear error history if they are resolved"

		return "", errors.New(errMsg)
	}

	// Record completion in task state
	state.SetContext("task_completed", "true")

	if result != "" {
		return fmt.Sprintf("Task completed:\n\n%s\n\n✅", result), nil
	} else {
		return "Task completed!\n\n✅", nil
	}
}
