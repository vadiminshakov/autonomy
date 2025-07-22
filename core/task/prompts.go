//nolint:all
package task

import (
	"github.com/vadiminshakov/autonomy/core/entity"
	"github.com/vadiminshakov/autonomy/core/tools"
)

const systemPrompt = `You are an AI coding assistant with access to powerful tools.

DECISION TREE (follow in order):
1. User requests a COMPLEX task (multiple files, analysis + modification) → ALWAYS use plan_execution first
2. User requests a task that requires 2+ tools → ALWAYS use plan_execution first
3. User requests a SIMPLE action (read one file, analyze one file) → Use the appropriate tool directly
4. User asks a CONCEPTUAL question → Provide explanation, then use attempt_completion
5. You receive a TOOL RESULT → Analyze result and decide: continue with more tools OR use attempt_completion
6. You have enough information to answer → Use attempt_completion immediately
7. You're UNSURE what to do → Use the most relevant tool to gather information

PLANNING GUIDELINES:
• MANDATORY: For any task that will require 3+ tools, use plan_execution FIRST - do NOT execute tools directly
• MANDATORY: For tasks that modify multiple files or require complex analysis, use plan_execution FIRST
• MANDATORY: For any refactoring, optimization, or system-wide changes, use plan_execution FIRST
• MANDATORY: If you think a task might need multiple steps or tools, err on the side of planning
• Simple tasks (read one file, analyze one file) don't need planning
• Examples of complex tasks requiring planning: "analyze all Go files", "refactor the codebase", "fix all issues", "optimize performance", "add new feature", "implement API changes"
• Examples of simple tasks: "read api.go", "analyze main.go", "what is this function"

PLANNING REQUIREMENT:
• When in doubt about complexity, ALWAYS choose plan_execution first
• Better to over-plan than to execute tools inefficiently
• The plan_execution tool will batch multiple related operations for better performance

CRITICAL COMPLETION RULES:
- ALWAYS use attempt_completion when you have sufficient information to answer the user's question
- Do NOT continue using tools indefinitely - be decisive about when to complete
- If you've analyzed the requested file(s) and can provide an assessment, use attempt_completion
- For analysis tasks: read file → analyze → attempt_completion (don't keep searching)
- For conceptual questions: explain → attempt_completion
- NEVER repeat the same tool with the same parameters multiple times
- When completing, ALWAYS provide a clear description of what was accomplished in the result field
- For complex multi-step tasks, consider using include_summary=true to provide detailed execution summary

CRITICAL RULES:
- For action requests, you MUST use tools - text-only responses are forbidden
- Trust the built-in repetition prevention - tools track their own usage automatically
- If a tool says "already used", don't retry it - use the previous results
- Before using expensive tools (get_project_structure, build_index), check if they were already used
- Use get_task_state or check_tool_usage to verify tool usage history when in doubt
- Stop when you have enough information to complete the task
- NEVER repeat the same tool with identical parameters multiple times

EFFICIENCY GUIDELINES:
• Focus on the specific task requested - do NOT expand scope without explicit permission
• Build on existing work rather than starting over
• When tools return "no matches found" repeatedly, stop searching and complete the task
• Use attempt_completion as soon as you can provide a meaningful answer

SCOPE FOCUS RULES:
• When user specifies ONE file - analyze ONLY that file, then complete
• When user says "изучи file.go" - read it, analyze it, then use attempt_completion
• Don't search for additional files unless explicitly requested
• Use attempt_completion when the requested scope is fully analyzed
• NEVER expand analysis beyond the single file mentioned in the request

Best Practices:
• Be efficient and direct in accomplishing tasks
• Complete tasks with attempt_completion as soon as you have sufficient information
• Don't over-analyze or search endlessly
• If searches return no results, conclude and complete the task

Error Recovery:
• If a tool fails with "already used" error, use previous results and complete the task
• Don't retry failed operations repeatedly
• The system will prevent harmful repetition automatically`

const forceToolsMessage = `You MUST use a tool. Your previous response had no tool calls.

Based on the user's request, execute one of these tools:
- For file operations: read_file, write_file, apply_diff
- For searching: search_dir, search_index, find_files
- For analysis: analyze_code_go, get_project_structure
- For execution: execute_command, go_test, go_vet
- For completion: attempt_completion
- For git operations: git_status, git_add, git_commit

Choose the most appropriate tool for the task and execute it NOW.`

func NewPromptData() *entity.PromptData {
	// map of concise tool descriptions understandable by language models
	toolDesc := map[string]string{
		"get_project_structure": "View project directory tree in a textual form",
		"read_file":             "Read file contents",
		"write_file":            "Create or overwrite a file with provided content",
		"apply_diff":            "Apply unified diff patch to modify existing file. Diff MUST be in proper format: hunk headers like '@@ -1,3 +1,4 @@', context lines with space prefix, removed lines with '-', added lines with '+'. Line numbers must match current file content exactly.",
		"execute_command":       "Run shell command in project root directory",
		"search_dir":            "Search text pattern recursively in directory",
		"find_files":            "Find files by glob pattern",
		"git_status":            "Show git status of working tree",
		"git_log":               "Show commit history",
		"git_diff":              "Show git diff of changes",
		"git_branch":            "Create, list or switch git branches",
		"attempt_completion":    "Mark task as finished and provide final summary. Optional: include_summary=true to add detailed execution summary",
		"post_request":          "Send HTTP POST request and return response",
		"get_request":           "Send HTTP GET request and return response",
		"copy_file":             "Copy file from source path to destination",
		"move_file":             "Move/Rename file",
		"delete_file":           "Delete file by path",
		"make_dir":              "Create directory",
		"remove_dir":            "Remove directory and its contents",
		"go_test":               "Run go test",
		"go_vet":                "Run go vet linter",
		"search_index":          "Search functions/classes/types in universal code index",
		"get_index_stats":       "Get statistics about universal code index",
		"get_function":          "Get detailed information about a code symbol",
		"get_type":              "Get detailed information about a code symbol",
		"get_package_info":      "Get information about a package/module",
		"analyze_code_go":       "Analyze Go code structure, complexity and provide recommendations",
		"rename_symbol_go":      "Rename a Go symbol (function, variable, type) throughout the file",
		"extract_function_go":   "Extract selected lines of Go code into a new function",
		"inline_function_go":    "Inline a Go function call by replacing it with function body",
		"get_task_state":        "Get current task execution state as JSON",
		"get_task_summary":      "Get human-readable summary of task progress",
		"reset_task_state":      "Reset task execution state",
		"check_tool_usage":      "Check if and how many times a specific tool has been used",
		"plan_execution":        "Create an execution plan for complex tasks with multiple tools",
	}

	var defs []entity.ToolDefinition

	for _, name := range tools.List() {
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

		case "get_task_state", "get_task_summary", "reset_task_state":
			schema["properties"] = map[string]any{}
			schema["required"] = []string{}

		case "check_tool_usage":
			schema["properties"] = map[string]any{
				"tool": map[string]string{"type": "string"},
			}
			schema["required"] = []string{"tool"}

		case "plan_execution":
			schema["properties"] = map[string]any{
				"task_description": map[string]string{"type": "string"},
				"tools_needed": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"tool":   map[string]string{"type": "string"},
							"args":   map[string]any{"type": "object"},
							"reason": map[string]string{"type": "string"},
						},
						"required": []string{"tool", "args", "reason"},
					},
				},
			}
			schema["required"] = []string{"task_description", "tools_needed"}

		case "attempt_completion":
			schema["properties"] = map[string]any{
				"result": map[string]string{
					"type":        "string",
					"description": "Description of what was accomplished",
				},
				"include_summary": map[string]any{
					"type":        "boolean",
					"description": "Whether to include detailed execution summary (optional, default: false)",
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

	return &entity.PromptData{
		SystemPrompt: systemPrompt,
		Messages:     []entity.Message{},
		Tools:        defs,
	}
}
