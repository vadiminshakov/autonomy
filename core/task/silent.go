package task

import (
	"fmt"
	"strings"
)

var silentTools = map[string]bool{
	"read_file":             true,
	"write_file":            true,
	"get_project_structure": true,
	"search_dir":            true,
	"find_files":            true,
	"dependency_analyzer":   true,
	"go_vet":                true,
}

func isSilentTool(name string) bool {
	return silentTools[name]
}

func silentToolSummary(toolName string, args map[string]any, result string) string {
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

	default:
		return ""
	}
}
