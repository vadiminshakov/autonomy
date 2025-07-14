package task

import (
	"autonomy/core/tools"
)

const systemPrompt = `You are an AI coding assistant. Solve tasks ONLY by calling tools.

Code Index Available:
• Multi-language support (Go, JavaScript, TypeScript, Python)
• Auto-updated at startup with file change detection
• Use search_index to find functions, classes, types across all languages
• Use get_function/get_type for detailed symbol information
• Use get_package_info for package/module overview

Rules:
1. Your very first response must be a tool call – no explanations or planning text.
2. Follow this workflow:
   • get_project_structure
   • search_index
   • read_file
   • execute_command (optional)
   • write_file / apply_diff
   • execute_command (build/test)
   • repeat until success
   • attempt_completion

Constraints:
• write production-ready code (no TODOs)
• after editing code always run tests and linter and fix all errors
• handle errors gracefully and provide clear error messages
• use timeouts and proper resource management
• implement proper security checks

Error handling:
• If a tool fails, analyze the error and try a different approach
• Don't retry the same failing operation multiple times
• Report meaningful error messages to the user

BEGIN NOW.`

const forceToolsMessage = `ERROR: You must use tools!

You have access to many tools like get_project_structure, read_file, write_file, execute_command, etc.
Use the appropriate tool to accomplish the task.

NO TEXT. TOOLS ONLY.`

type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema,omitempty"`
}

type PromptData struct {
	SystemPrompt string
	Messages     []Message
	Tools        []ToolDefinition
}

func NewPromptData() *PromptData {
	// map of concise tool descriptions understandable by language models
	toolDesc := map[string]string{
		"get_project_structure": "View project directory tree in a textual form",
		"read_file":             "Read file contents",
		"write_file":            "Create or overwrite a file with provided content",
		"apply_diff":            "Apply unified diff patch to modify existing file",
		"execute_command":       "Run shell command in project root directory",
		"search_dir":            "Search text pattern recursively in directory",
		"find_files":            "Find files by glob pattern",
		"git_status":            "Show git status of working tree",
		"git_add":               "Stage files for git commit",
		"git_commit":            "Create git commit with message",
		"git_log":               "Show commit history",
		"git_diff":              "Show git diff of changes",
		"git_branch":            "Create, list or switch git branches",
		"attempt_completion":    "Mark task as finished and provide final summary",
		"post_request":          "Send HTTP POST request and return response",
		"get_request":           "Send HTTP GET request and return response",
		"calc":                  "Evaluate arithmetic expression and return the result",
		"copy_file":             "Copy file from source path to destination",
		"move_file":             "Move/Rename file",
		"delete_file":           "Delete file by path",
		"make_dir":              "Create directory",
		"remove_dir":            "Remove directory and its contents",
		"dependency_analyzer":   "Analyze Go module dependencies",
		"go_test":               "Run go test",
		"go_vet":                "Run go vet linter",
		"build_index":           "Rebuild universal code index (Go/JS/TS/Python)",
		"search_index":          "Search functions/classes/types in universal code index",
		"get_index_stats":       "Get statistics about universal code index",
		"get_function":          "Get detailed information about a code symbol",
		"get_type":              "Get detailed information about a code symbol",
		"get_package_info":      "Get information about a package/module",
	}

	var defs []ToolDefinition

	for _, name := range tools.List() {
		desc, ok := toolDesc[name]
		if !ok {
			desc = "Internal tool " + name
		}

		schema := map[string]interface{}{
			"type": "object",
		}

		switch name {
		case "read_file":
			schema["properties"] = map[string]interface{}{
				"path": map[string]string{"type": "string"},
			}
			schema["required"] = []string{"path"}

		case "write_file":
			schema["properties"] = map[string]interface{}{
				"path":    map[string]string{"type": "string"},
				"content": map[string]string{"type": "string"},
			}
			schema["required"] = []string{"path", "content"}

		case "apply_diff":
			schema["properties"] = map[string]interface{}{
				"path": map[string]string{"type": "string"},
				"diff": map[string]string{"type": "string"},
			}
			schema["required"] = []string{"path", "diff"}

		case "execute_command":
			schema["properties"] = map[string]interface{}{
				"command": map[string]string{"type": "string"},
			}
			schema["required"] = []string{"command"}

		case "search_dir":
			schema["properties"] = map[string]interface{}{
				"path":  map[string]string{"type": "string"},
				"query": map[string]string{"type": "string"},
			}
			schema["required"] = []string{"query"}

		case "find_files":
			schema["properties"] = map[string]interface{}{
				"pattern": map[string]string{"type": "string"},
			}
			schema["required"] = []string{"pattern"}

		case "calc":
			schema["properties"] = map[string]interface{}{
				"expression": map[string]string{"type": "string"},
			}
			schema["required"] = []string{"expression"}

		case "dependency_analyzer":
			schema["properties"] = map[string]interface{}{
				"action": map[string]interface{}{"type": "string", "enum": []string{"list", "outdated", "graph"}},
			}
			schema["required"] = []string{"action"}

		case "go_test":
			schema["properties"] = map[string]interface{}{
				"path": map[string]string{"type": "string"},
			}
			schema["required"] = []string{}

		case "go_vet":
			schema["properties"] = map[string]interface{}{
				"path": map[string]string{"type": "string"},
			}
			schema["required"] = []string{}

		case "build_index":
			schema["properties"] = map[string]interface{}{
				"path": map[string]string{"type": "string"},
			}
			schema["required"] = []string{}

		case "search_index":
			schema["properties"] = map[string]interface{}{
				"query": map[string]string{"type": "string"},
			}
			schema["required"] = []string{"query"}

		case "get_function":
			schema["properties"] = map[string]interface{}{
				"key": map[string]string{"type": "string"},
			}
			schema["required"] = []string{"key"}

		case "get_type":
			schema["properties"] = map[string]interface{}{
				"key": map[string]string{"type": "string"},
			}
			schema["required"] = []string{"key"}

		case "get_package_info":
			schema["properties"] = map[string]interface{}{
				"package": map[string]string{"type": "string"},
			}
			schema["required"] = []string{"package"}

		default:
			schema["properties"] = map[string]interface{}{}
			schema["required"] = []string{}
		}

		defs = append(defs, ToolDefinition{
			Name:        name,
			Description: desc,
			InputSchema: schema,
		})
	}

	return &PromptData{
		SystemPrompt: systemPrompt,
		Messages:     []Message{},
		Tools:        defs,
	}
}

func (p *PromptData) AddMessage(role, content string) {
	p.Messages = append(p.Messages, Message{Role: role, Content: content})
}

func (p *PromptData) GetForceToolsMessage() string {
	return forceToolsMessage
}
