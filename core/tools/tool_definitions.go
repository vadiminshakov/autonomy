package tools

import (
	"github.com/vadiminshakov/autonomy/core/entity"
)

// GetToolDescriptions returns tool definitions
//
//nolint:lll,gocyclo
func GetToolDescriptions() []entity.ToolDefinition {
	toolDesc := map[string]string{
		"get_project_structure": "View project directory tree in a textual form. Use to understand project layout",
		"read_file":             "Read file contents",
		"write_file":            "Create a NEW file or FULLY REPLACE an entire file ONLY when explicitly instructed. Do NOT use for partial edits. If the file exists and only changes are needed, use lsp_edit instead",
		"lsp_edit":              "Modify EXISTING files with precise line-based edits (insert/replace/delete). Supports multiple edits in a single call. This is the DEFAULT tool for any modifications",
		"search_dir":            "Search text pattern recursively in directory",
		"find_files":            "Find files by glob pattern. Use before read_file to verify file exists",
		"bash":                  "Execute any bash command. Replaces git, file operations, and directory commands",
		"attempt_completion":    "Mark task as finished and provide completion result. Use ONLY when task is fully completed",
		"get_task_state":        "Get current task execution state as JSON. Use to track what has been done",
		"reset_task_state":      "Reset task execution state. Use carefully",
		"check_tool_usage":      "Check if and how many times a specific tool has been used",
		"decompose_task":        "Task decomposition: breaks complex tasks into executable steps using intelligent analysis. Use for multi-step tasks",
		"interrupt_command":     "Execute command with interrupt capability - automatically stops long-running commands after 10s and analyzes their output",
	}

	var defs []entity.ToolDefinition

	for _, name := range List() {
		desc, ok := toolDesc[name]
		if !ok {
			desc = "Internal tool " + name
		}

		schema := map[string]any{
			"type": "object",
		}

		switch name {
		case "read_file":
			schema["properties"] = map[string]any{
				"path": map[string]string{"type": "string"},
			}
			schema["required"] = []string{"path"}

		case "write_file":
			schema["properties"] = map[string]any{
				"path":    map[string]string{"type": "string"},
				"content": map[string]string{"type": "string"},
			}
			schema["required"] = []string{"path", "content"}

		case "lsp_edit":
			schema["properties"] = map[string]any{
				"path": map[string]string{"type": "string"},
				"edits": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"start_line":  map[string]string{"type": "integer"},
							"end_line":    map[string]string{"type": "integer"},
							"new_text":    map[string]string{"type": "string"},
							"description": map[string]string{"type": "string"},
						},
						"required": []string{"start_line", "end_line", "new_text"},
					},
				},
			}
			schema["required"] = []string{"path", "edits"}

		case "bash":
			schema["properties"] = map[string]any{
				"command": map[string]string{"type": "string"},
			}
			schema["required"] = []string{"command"}

		case "search_dir":
			schema["properties"] = map[string]any{
				"path":  map[string]string{"type": "string"},
				"query": map[string]string{"type": "string"},
			}
			schema["required"] = []string{"query"}

		case "find_files":
			schema["properties"] = map[string]any{
				"pattern": map[string]string{"type": "string"},
			}
			schema["required"] = []string{"pattern"}

		case "get_task_state", "reset_task_state":
			schema["properties"] = map[string]any{}
			schema["required"] = []string{}

		case "interrupt_command":
			schema["properties"] = map[string]any{
				"command": map[string]string{
					"type":        "string",
					"description": "Command to execute with interrupt capability",
				},
			}
			schema["required"] = []string{"command"}

		case "check_tool_usage":
			schema["properties"] = map[string]any{
				"tool": map[string]string{"type": "string"},
			}
			schema["required"] = []string{"tool"}

		case "decompose_task":
			schema["properties"] = map[string]any{
				"task_description": map[string]any{
					"type":        "string",
					"description": "Detailed description of the complex task to be broken down into steps",
				},
			}
			schema["required"] = []string{"task_description"}

		case "attempt_completion":
			schema["properties"] = map[string]any{
				"result": map[string]string{
					"type":        "string",
					"description": "Optional description of what was accomplished",
				},
			}
			schema["required"] = []string{}

		default:
			schema["properties"] = map[string]any{}
			schema["required"] = []string{}
		}

		defs = append(defs, entity.ToolDefinition{
			Name:        name,
			Description: desc,
			InputSchema: schema,
		})
	}

	return defs
}
