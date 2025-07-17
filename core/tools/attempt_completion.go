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

	includeSummary := false
	if enableSummary, ok := args["include_summary"].(bool); ok && enableSummary {
		includeSummary = true
	}

	if includeSummary {
		taskState := getTaskState()
		summary := taskState.GetSummary()

		if result == "" {
			return fmt.Sprintf("🎉 Task completed\n\n%s", summary), nil
		}
		return fmt.Sprintf("🎉 Task completed:\n%s\n\n%s", result, summary), nil
	}

	if result == "" {
		return "🎉 Task completed", nil
	}
	return fmt.Sprintf("🎉 Task completed:\n%s", result), nil
}
