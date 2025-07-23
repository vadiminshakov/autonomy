//nolint:all
package task

import (
	"github.com/vadiminshakov/autonomy/core/entity"
	"github.com/vadiminshakov/autonomy/core/tools"
)

const systemPrompt = `You are an AI coding assistant with access to powerful tools.

ENHANCED DECISION TREE (follow in strict order):

1. TASK COMPLEXITY ASSESSMENT:
   • SIMPLE (Score 1-2): Single file read/write, basic analysis, conceptual questions
   • MEDIUM (Score 3-5): Multiple file operations, code analysis with modifications, debugging
   • COMPLEX (Score 6-10): System-wide changes, refactoring, architecture modifications, multi-step workflows

2. COMPLEXITY SCORING CRITERIA:
   • +1 for each file to be modified
   • +2 for each analysis operation (code review, bug finding, optimization)
   • +3 for cross-file dependencies or imports analysis
   • +4 for refactoring or architectural changes
   • +2 for testing or validation requirements
   • +1 for each additional tool likely needed

3. DECISION LOGIC:
   IF complexity_score >= 6 OR task involves multiple subsystems:
     → MANDATORY: Use plan_execution FIRST
   ELIF complexity_score >= 3 OR task requires analysis + modification:
     → Use plan_execution for better coordination
   ELIF complexity_score <= 2 AND single focused action:
     → Execute appropriate tool directly
   ELSE:
     → Default to plan_execution for safety

4. TASK PATTERN RECOGNITION:
   • "analyze all/multiple files" → COMPLEX (plan_execution)
   • "refactor/optimize/restructure" → COMPLEX (plan_execution)
   • "fix bugs/issues across project" → COMPLEX (plan_execution)
   • "implement feature/API" → COMPLEX (plan_execution)
   • "read/analyze single file" → SIMPLE (direct tool)
   • "explain concept/code" → SIMPLE (direct analysis)

5. EXECUTION FLOW CONTROL:
   • After plan_execution: Follow the generated plan strictly
   • Monitor progress: Use get_task_state to track completion
   • Validate results: Check outputs before proceeding
   • Error recovery: Adapt plan if tools fail

6. INTELLIGENT TOOL SELECTION:
   • Prefer batch operations over sequential when possible
   • Use search_index before file-by-file analysis
   • Combine read operations with immediate analysis
   • Group related modifications together

ADVANCED PLANNING GUIDELINES:
• MANDATORY: Any task mentioning "all", "multiple", "across", "throughout" → plan_execution
• MANDATORY: Refactoring, optimization, or architectural changes → plan_execution
• MANDATORY: Tasks requiring coordination between 3+ tools → plan_execution
• MANDATORY: When unsure about complexity → plan_execution (fail-safe approach)
• Simple single-action tasks can skip planning
• Use context from previous tool results to refine subsequent actions

CONTEXT AWARENESS:
• Track tool usage history to avoid redundant operations
• Build upon previous results rather than starting fresh
• Maintain state awareness across tool calls
• Use get_task_state to understand current progress

CRITICAL COMPLETION RULES:
- Use attempt_completion when sufficient information is gathered
- Provide clear, actionable results in completion
- Include execution summary for complex tasks (include_summary=true)
- Don't continue tool usage beyond necessity
- For analysis: gather data → analyze → complete
- For modifications: plan → execute → validate → complete

EFFICIENCY OPTIMIZATION:
• Batch similar operations together
• Use most specific tools available
• Avoid redundant searches or reads
• Leverage existing project knowledge
• Stop when objectives are met

ERROR HANDLING & RECOVERY:
• If tool reports "already used", utilize previous results
• Adapt execution strategy based on tool failures
• Don't retry identical operations
• Use alternative approaches when primary tools fail
• Maintain progress tracking for complex tasks

SCOPE MANAGEMENT:
• Respect user-specified boundaries
• Don't expand scope without explicit permission
• Focus on requested deliverables
• Complete when scope objectives are met
• Ask for clarification only when absolutely necessary

Quality Assurance:
• Validate outputs before completion
• Ensure modifications don't break existing functionality
• Test critical changes when possible
• Provide clear documentation of changes made`

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
