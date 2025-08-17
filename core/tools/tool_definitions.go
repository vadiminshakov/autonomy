package tools

import (
	"github.com/vadiminshakov/autonomy/core/entity"
)

// GetToolDescriptions returns tool definitions
//
//nolint:lll,gocyclo
func GetToolDescriptions() []entity.ToolDefinition {
	// map of concise tool descriptions understandable by language models
	toolDesc := map[string]string{
		"get_project_structure": "View project directory tree in a textual form. Use before starting work to understand project layout",
		"read_file":             "Read file contents. Example: {\"path\": \"main.go\"} to read main.go file",
		"write_file":            "Create or overwrite a file with provided content. Example: {\"path\": \"test.txt\", \"content\": \"Hello World\"} - ALWAYS read existing file first if modifying",
		"apply_diff":            "Apply unified diff patch to modify existing file. Diff MUST be in proper format: hunk headers like '@@ -1,3 +1,4 @@', context lines with space prefix, removed lines with '-', added lines with '+'. Line numbers must match current file content exactly. Use after read_file to ensure correct line numbers",
		"execute_command":       "Run shell command in project root directory. Example: {\"command\": \"go run main.go\"} - output will be returned",
		"search_dir":            "Search text pattern recursively in directory. Example: {\"query\": \"func main\", \"path\": \".\"} to find all main functions",
		"find_files":            "Find files by glob pattern. Example: {\"pattern\": \"*.go\"} to find all Go files. Use before read_file to verify file exists",
		"git_status":            "Show git status of working tree. No parameters needed. Use to check what files were modified",
		"git_log":               "Show commit history. Optional parameters for filtering",
		"git_diff":              "Show git diff of changes. Use to see exact modifications made",
		"git_branch":            "Create, list or switch git branches",
		"attempt_completion":    "Mark task as finished and provide final summary. Use ONLY when task is fully completed. Example: {\"message\": \"Task completed: Created REST API with CRUD operations\"}",
		"post_request":          "Send HTTP POST request and return response",
		"get_request":           "Send HTTP GET request and return response",
		"copy_file":             "Copy file from source path to destination. Example: {\"source\": \"file1.txt\", \"destination\": \"backup/file1.txt\"}",
		"move_file":             "Move/Rename file. Example: {\"source\": \"old.txt\", \"destination\": \"new.txt\"}",
		"delete_file":           "Delete file by path. Example: {\"path\": \"temp.txt\"}",
		"make_dir":              "Create directory. Example: {\"path\": \"src/components\", \"recursive\": true} for nested directories",
		"remove_dir":            "Remove directory and its contents. Use with caution",
		"go_test":               "Run go test. Example: {\"path\": \"./...\"} to test all packages",
		"go_vet":                "Run go vet linter. Example: {\"path\": \".\"} to check current directory",
		"search_index":          "Search functions/classes/types in universal code index. Example: {\"query\": \"NewServer\"} to find NewServer function",
		"get_index_stats":       "Get statistics about universal code index. No parameters needed",
		"get_function":          "Get detailed information about a code symbol. Example: {\"key\": \"package/file.go:FunctionName\"}",
		"get_type":              "Get detailed information about a code symbol",
		"get_package_info":      "Get information about a package/module. Example: {\"package\": \"fmt\"}",
		"analyze_code_go":       "Analyze Go code structure, complexity and provide recommendations. Example: {\"path\": \"main.go\"}",
		"rename_symbol_go":      "Rename a Go symbol (function, variable, type) throughout the file. Example: {\"file\": \"main.go\", \"old_name\": \"oldFunc\", \"new_name\": \"newFunc\"}",
		"extract_function_go":   "Extract selected lines of Go code into a new function",
		"inline_function_go":    "Inline a Go function call by replacing it with function body",
		"get_task_state":        "Get current task execution state as JSON. Use to track what has been done",
		"reset_task_state":      "Reset task execution state. Use carefully",
		"check_tool_usage":      "Check if and how many times a specific tool has been used. Example: {\"tool_name\": \"read_file\"}",
		"decompose_task":        "Task decomposition: breaks complex tasks into executable steps using intelligent analysis. Use for multi-step tasks. Example: {\"task_description\": \"Create REST API with CRUD operations\"}",
		"interrupt_command":     "Execute command with interrupt capability - automatically stops long-running commands after 10s and analyzes their output. Example: {\"command\": \"go run server.go\"} for testing servers",
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

		case "apply_diff":
			schema["properties"] = map[string]any{
				"path": map[string]string{"type": "string"},
				"diff": map[string]string{"type": "string"},
			}
			schema["required"] = []string{"path", "diff"}

		case "execute_command":
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

		case "go_test":
			schema["properties"] = map[string]any{
				"path": map[string]string{"type": "string"},
			}
			schema["required"] = []string{}

		case "go_vet":
			schema["properties"] = map[string]any{
				"path": map[string]string{"type": "string"},
			}
			schema["required"] = []string{}

		case "search_index":
			schema["properties"] = map[string]any{
				"query": map[string]string{"type": "string"},
			}
			schema["required"] = []string{"query"}

		case "get_function":
			schema["properties"] = map[string]any{
				"key": map[string]string{"type": "string"},
			}
			schema["required"] = []string{"key"}

		case "get_type":
			schema["properties"] = map[string]any{
				"key": map[string]string{"type": "string"},
			}
			schema["required"] = []string{"key"}

		case "get_package_info":
			schema["properties"] = map[string]any{
				"package": map[string]string{"type": "string"},
			}
			schema["required"] = []string{"package"}

		case "analyze_code_go":
			schema["properties"] = map[string]any{
				"path": map[string]string{"type": "string"},
			}
			schema["required"] = []string{"path"}

		case "rename_symbol_go":
			schema["properties"] = map[string]any{
				"file":     map[string]string{"type": "string"},
				"old_name": map[string]string{"type": "string"},
				"new_name": map[string]string{"type": "string"},
			}
			schema["required"] = []string{"file", "old_name", "new_name"}

		case "extract_function_go":
			schema["properties"] = map[string]any{
				"file":          map[string]string{"type": "string"},
				"start_line":    map[string]string{"type": "number"},
				"end_line":      map[string]string{"type": "number"},
				"function_name": map[string]string{"type": "string"},
			}
			schema["required"] = []string{"file", "start_line", "end_line", "function_name"}

		case "inline_function_go":
			schema["properties"] = map[string]any{
				"file":          map[string]string{"type": "string"},
				"function_name": map[string]string{"type": "string"},
				"line_number":   map[string]string{"type": "number"},
			}
			schema["required"] = []string{"file", "function_name", "line_number"}

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
					"description": "Description of what was accomplished",
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
