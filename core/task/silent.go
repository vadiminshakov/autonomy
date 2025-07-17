package task

import (
	"fmt"
	"strings"
)

// Tools considered "silent" – output is shown in truncated form
var silentTools = map[string]bool{
	"read_file":             true,
	"write_file":            true,
	"get_project_structure": true,
	"search_dir":            true,
	"find_files":            true,
	"dependency_analyzer":   true,
	"go_vet":                true,
	"plan_execution":        true,
}

// isSilentTool checks if tool output should be summarized
func isSilentTool(name string) bool {
	return silentTools[name]
}

// silentToolSummary generates a concise summary for silent tool results
func silentToolSummary(toolName string, args map[string]interface{}, result string) string {
	switch toolName {
	case "read_file":
		if path, ok := args["path"].(string); ok {
			return fmt.Sprintf(" - read file %s", path)
		}

		return " - file read"

	case "write_file":
		if path, ok := args["path"].(string); ok {
			return fmt.Sprintf(" - created file %s", path)
		}

		return " - file created"

	case "get_project_structure":
		lines := strings.Count(result, "\n")
		if path, ok := args["path"].(string); ok && path != "" {
			return fmt.Sprintf(" - structure of %s (%d items)", path, lines)
		}

		return fmt.Sprintf(" - project structure (%d items)", lines)

	case "search_dir":
		if strings.Contains(result, "no matches found") {
			return " - no matches found"
		}

		for _, line := range strings.Split(result, "\n") {
			if strings.Contains(line, "Found") && strings.Contains(line, "matches") {
				return " - " + strings.ToLower(line)
			}
		}

		return " - search completed"

	case "find_files":
		if strings.Contains(result, "no files found") {
			return " - no files found"
		}

		for _, line := range strings.Split(result, "\n") {
			if strings.Contains(line, "Found") && strings.Contains(line, "files") {
				return " - " + strings.ToLower(line)
			}
		}

		return " - file search completed"

	case "dependency_analyzer":
		return " - dependency analysis done"

	case "go_vet":
		if strings.Contains(result, "no issues") {
			return " - vet clean"
		}

		return " - vet issues"

	case "plan_execution":
		if strings.Contains(result, "Execution Plan for:") {
			// extract the task description
			startIdx := strings.Index(result, "Execution Plan for: ")
			if startIdx >= 0 {
				startIdx += 19 // Length of "Execution Plan for: "
				endIdx := strings.Index(result[startIdx:], "\n")
				if endIdx >= 0 {
					taskDesc := result[startIdx : startIdx+endIdx]
					return fmt.Sprintf(" - created plan for: %s", taskDesc)
				}
			}

			// extract step count
			stepsIdx := strings.Index(result, "Total steps: ")
			if stepsIdx >= 0 {
				stepsIdx += 13 // Length of "Total steps: "
				endIdx := strings.Index(result[stepsIdx:], "\n")
				if endIdx >= 0 {
					steps := result[stepsIdx : stepsIdx+endIdx]
					return fmt.Sprintf(" - created plan with %s steps", steps)
				}
			}
		}

		return " - execution plan created"

	default:
		return ""
	}
}
