package tools

import (
	"fmt"
	"os"
)

type ToolFunc func(args map[string]interface{}) (string, error)

var registry = make(map[string]ToolFunc)

// ClearRegistry clears the registry (mainly for testing)
func ClearRegistry() {
	registry = make(map[string]ToolFunc)
}

func Register(name string, fn ToolFunc) {
	registry[name] = fn
}

func Execute(name string, args map[string]interface{}) (string, error) {
	fn, ok := registry[name]
	if !ok {
		// suggest similar tool names if available
		suggestions := getSimilarToolNames(name)
		if len(suggestions) > 0 {
			return "", fmt.Errorf("tool %s not found. did you mean: %v", name, suggestions)
		}
		return "", fmt.Errorf("tool %s not found. use 'get_task_state' to see available tools", name)
	}

	// логируем входные параметры для отладки (кроме больших значений)
	if debugMode := getDebugMode(); debugMode {
		logToolCall(name, args)
	}

	// validate common required parameters before execution
	if err := validateToolArgs(name, args); err != nil {
		return "", err
	}

	result, err := fn(args)

	// record tool usage in task state (except for task state tools themselves to avoid recursion)
	if name != "get_task_state" &&
		name != "check_tool_usage" && name != "reset_task_state" {
		success := err == nil
		getTaskState().RecordToolUse(name, success, result)
	}

	return result, err
}

// validateToolArgs performs basic validation of tool arguments
func validateToolArgs(toolName string, args map[string]interface{}) error {
	// Basic validation for common tools
	switch toolName {
	case "read_file", "write_file", "lsp_edit", "delete_file", "copy_file", "move_file":
		if _, ok := args["path"]; !ok {
			return fmt.Errorf("tool %s requires 'path' parameter. example: {\"path\": \"file.txt\"}", toolName)
		}
	case "execute_command", "interrupt_command":
		if _, ok := args["command"]; !ok {
			return fmt.Errorf("tool %s requires 'command' parameter. example: {\"command\": \"go run main.go\"}", toolName)
		}
	case "search_dir", "search_index":
		if _, ok := args["query"]; !ok {
			return fmt.Errorf("tool %s requires 'query' parameter. example: {\"query\": \"func main\"}", toolName)
		}
	case "find_files":
		if _, ok := args["pattern"]; !ok {
			return fmt.Errorf("tool %s requires 'pattern' parameter. example: {\"pattern\": \"*.go\"}", toolName)
		}
	case "make_dir", "remove_dir":
		if _, ok := args["path"]; !ok {
			return fmt.Errorf("tool %s requires 'path' parameter. example: {\"path\": \"src/components\"}", toolName)
		}
	}
	return nil
}

// getSimilarToolNames returns tool names similar to the given name
func getSimilarToolNames(name string) []string {
	var suggestions []string
	allTools := List()

	// Simple similarity check - could be improved with edit distance
	for _, tool := range allTools {
		// Check if tool contains the name or vice versa
		if len(name) > 3 && (contains(tool, name) || contains(name, tool)) {
			suggestions = append(suggestions, tool)
		}
	}

	if len(suggestions) > 3 {
		suggestions = suggestions[:3] // Limit to 3 suggestions
	}

	return suggestions
}

// Helper function for case-insensitive contains check
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			len(s) > len(substr) &&
				(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr))
}

// getDebugMode проверяет включен ли режим отладки
func getDebugMode() bool {
	// можно включить через переменную окружения AUTONOMY_DEBUG=true
	return os.Getenv("AUTONOMY_DEBUG") == "true"
}

// logToolCall логирует вызов инструмента с параметрами
func logToolCall(name string, args map[string]interface{}) {
	// ограничиваем размер логируемых значений
	logArgs := make(map[string]interface{})
	for k, v := range args {
		if str, ok := v.(string); ok && len(str) > 200 {
			logArgs[k] = str[:200] + "... (truncated)"
		} else {
			logArgs[k] = v
		}
	}

	// записываем в stderr чтобы не смешивать с выводом программы
	fmt.Fprintf(os.Stderr, "[DEBUG] Tool call: %s with args: %+v\n", name, logArgs)
}

func List() []string {
	keys := make([]string, 0, len(registry))
	for k := range registry {
		keys = append(keys, k)
	}
	return keys
}
