package tools

import (
	"fmt"
)

type ToolFunc func(args map[string]interface{}) (string, error)

var registry = make(map[string]ToolFunc)

func Register(name string, fn ToolFunc) {
	registry[name] = fn
}

func Execute(name string, args map[string]interface{}) (string, error) {
	fn, ok := registry[name]
	if !ok {
		return "", fmt.Errorf("tool %s not found", name)
	}

	result, err := fn(args)

	// record tool usage in task state (except for task state tools themselves to avoid recursion)
	if name != "get_task_state" && name != "get_task_summary" &&
		name != "check_tool_usage" && name != "reset_task_state" {
		success := err == nil
		getTaskState().RecordToolUse(name, success, result)
	}

	return result, err
}

func List() []string {
	keys := make([]string, 0, len(registry))
	for k := range registry {
		keys = append(keys, k)
	}
	return keys
}
