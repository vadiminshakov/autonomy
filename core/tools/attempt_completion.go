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

	// Record completion in task state
	state := getTaskState()
	state.SetContext("task_completed", "true")

	if result == "" {
		return "🎉 TASK COMPLETED SUCCESSFULLY\n\n✅ All requirements have been fulfilled.\nNo further action is required.", nil
	}

	return fmt.Sprintf("🎉 TASK COMPLETED SUCCESSFULLY\n\n%s\n\n✅ All requirements have been fulfilled.\nNo further action is required.", result), nil
}
