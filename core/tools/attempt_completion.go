package tools

import (
	"fmt"
)

func init() {
	Register("attempt_completion", AttemptCompletion)
}

// AttemptCompletion marks the task as completed and returns a final message.
func AttemptCompletion(args map[string]interface{}) (string, error) {
	result, _ := args["result"].(string)

	// check required parameter
	if result == "" {
		return "", fmt.Errorf("parameter 'result' is required for attempt_completion")
	}

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

		return "", fmt.Errorf(errMsg)
	}

	// Record completion in task state
	state.SetContext("task_completed", "true")

	return fmt.Sprintf("🎉 Task completed:\n\n%s\n\n✅ All requirements have been fulfilled.\nNo further action is required.", result), nil
}
