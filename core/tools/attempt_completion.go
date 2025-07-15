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
	if result == "" {
		return "🎉 Task completed", nil
	}
	return fmt.Sprintf("🎉 Task completed:\n%s", result), nil
}
