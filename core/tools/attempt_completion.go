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
	state := getTaskState()

	// only check if the last tool was successful - ignore historical errors
	if !state.LastToolSucceeded() {
		return "", errors.New("cannot complete task: last operation failed")
	}

	// check if we're in a decomposed task execution
	hasTask, exists := state.GetContext("has_decomposed_task")
	isDecomposedTask := exists && hasTask == true

	// for decomposed tasks, only set step completion flag, don't terminate
	if isDecomposedTask {
		state.SetContext("step_completed", "true")
		if result != "" {
			return fmt.Sprintf("Step completed:\n%s\n✅", result), nil
		} else {
			return "Step completed!\n✅", nil
		}
	}

	// for direct tasks, mark as fully completed
	state.SetContext("task_completed", "true")

	if result != "" {
		return fmt.Sprintf("Task completed:\n\n%s\n\n✅", result), nil
	} else {
		return "Task completed!\n\n✅", nil
	}
}
