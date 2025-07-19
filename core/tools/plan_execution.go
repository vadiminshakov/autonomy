package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vadiminshakov/autonomy/core/entity"
)

func init() {
	Register("plan_execution", PlanExecution)
}

// PlanExecution creates an execution plan for complex tasks
func PlanExecution(args map[string]interface{}) (string, error) {
	taskDesc, ok := args["task_description"].(string)
	if !ok || taskDesc == "" {
		return "", fmt.Errorf("parameter 'task_description' must be a non-empty string")
	}

	toolsNeededRaw, ok := args["tools_needed"]
	if !ok {
		return "", fmt.Errorf("parameter 'tools_needed' is required")
	}

	toolCalls, err := parseToolsNeeded(toolsNeededRaw)
	if err != nil {
		return "", fmt.Errorf("failed to parse tools_needed: %v", err)
	}

	if len(toolCalls) == 0 {
		return "", fmt.Errorf("at least one tool must be specified in tools_needed")
	}

	planData := map[string]any{
		"task_description": taskDesc,
		"tool_calls":       toolCalls,
	}

	planJSON, err := json.Marshal(planData)
	if err != nil {
		return "", fmt.Errorf("failed to serialize plan: %v", err)
	}

	// store in task state
	state := getTaskState()
	state.SetContext("has_execution_plan", true)
	state.SetContext("execution_plan", string(planJSON))

	summary := generatePlanSummary(taskDesc, toolCalls)

	return summary, nil
}

// parseToolsNeeded converts the tools_needed argument to a list of entity.ToolCall
//
//nolint:gocyclo
func parseToolsNeeded(toolsNeededRaw any) ([]entity.ToolCall, error) {
	var toolCalls []entity.ToolCall

	// handle different input formats
	switch v := toolsNeededRaw.(type) {
	case []any:
		// array of tool specifications
		for i, toolRaw := range v {
			toolMap, ok := toolRaw.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("tool at index %d must be an object", i)
			}

			// extract tool name (support both "name" and "tool" fields)
			var name string
			if nameRaw, ok := toolMap["name"]; ok {
				if nameStr, ok := nameRaw.(string); ok && nameStr != "" {
					name = nameStr
				}
			}
			if name == "" {
				if toolRaw, ok := toolMap["tool"]; ok {
					if toolStr, ok := toolRaw.(string); ok && toolStr != "" {
						name = toolStr
					}
				}
			}
			if name == "" {
				return nil, fmt.Errorf("tool at index %d missing name/tool field", i)
			}

			// extract arguments (support both "args" and "arguments" fields)
			var args map[string]any
			if argsRaw, ok := toolMap["args"]; ok {
				if argsMap, ok := argsRaw.(map[string]any); ok {
					args = argsMap
				}
			}
			if args == nil {
				if argsRaw, ok := toolMap["arguments"]; ok {
					if argsMap, ok := argsRaw.(map[string]any); ok {
						args = argsMap
					}
				}
			}
			if args == nil {
				args = make(map[string]any)
			}

			toolCalls = append(toolCalls, entity.ToolCall{
				Name: name,
				Args: args,
			})
		}

	case string:
		// try to parse as JSON
		var tools []map[string]any
		if err := json.Unmarshal([]byte(v), &tools); err != nil {
			return nil, fmt.Errorf("failed to parse tools_needed as JSON: %v", err)
		}

		// convert the parsed JSON
		for i, toolMap := range tools {
			// extract tool name
			var name string
			if nameRaw, ok := toolMap["name"]; ok {
				if nameStr, ok := nameRaw.(string); ok && nameStr != "" {
					name = nameStr
				}
			}
			if name == "" {
				if toolRaw, ok := toolMap["tool"]; ok {
					if toolStr, ok := toolRaw.(string); ok && toolStr != "" {
						name = toolStr
					}
				}
			}
			if name == "" {
				return nil, fmt.Errorf("tool at index %d missing name/tool field", i)
			}

			// extract arguments
			var args map[string]any
			if argsRaw, ok := toolMap["args"]; ok {
				if argsMap, ok := argsRaw.(map[string]any); ok {
					args = argsMap
				}
			}
			if args == nil {
				if argsRaw, ok := toolMap["arguments"]; ok {
					if argsMap, ok := argsRaw.(map[string]any); ok {
						args = argsMap
					}
				}
			}
			if args == nil {
				args = make(map[string]any)
			}

			toolCalls = append(toolCalls, entity.ToolCall{
				Name: name,
				Args: args,
			})
		}

	default:
		return nil, fmt.Errorf("tools_needed must be an array of tool specifications")
	}

	return toolCalls, nil
}

// generatePlanSummary creates a human-readable summary of the execution plan
func generatePlanSummary(taskDesc string, toolCalls []entity.ToolCall) string {
	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("Execution Plan for: %s\n\n", taskDesc))
	summary.WriteString(fmt.Sprintf("Total steps: %d\n\n", len(toolCalls)))

	for i, call := range toolCalls {
		summary.WriteString(fmt.Sprintf("Step %d: %s\n", i+1, call.Name))

		if len(call.Args) > 0 {
			summary.WriteString("  Arguments:\n")
			for k, v := range call.Args {
				summary.WriteString(fmt.Sprintf("    %s: %v\n", k, v))
			}
		}

		summary.WriteString("\n")
	}

	summary.WriteString("\nPlan has been stored and will be executed by the main task loop.")
	return summary.String()
}

// HasStoredToolCalls checks if there are stored tool calls for execution
func HasStoredToolCalls() bool {
	state := getTaskState()
	hasPlan, exists := state.GetContext("has_execution_plan")
	return exists && hasPlan == true
}

// GetStoredToolCalls retrieves the stored tool calls and task description
func GetStoredToolCalls() (string, []entity.ToolCall, error) {
	state := getTaskState()

	hasPlan, exists := state.GetContext("has_execution_plan")
	if !exists || hasPlan != true {
		return "", nil, fmt.Errorf("no execution plan found")
	}

	planDataRaw, exists := state.GetContext("execution_plan")
	if !exists {
		return "", nil, fmt.Errorf("execution plan flag set but no plan data found")
	}

	planDataStr, ok := planDataRaw.(string)
	if !ok {
		return "", nil, fmt.Errorf("execution plan data is not a string")
	}

	var planData map[string]any
	if err := json.Unmarshal([]byte(planDataStr), &planData); err != nil {
		return "", nil, fmt.Errorf("failed to parse execution plan: %v", err)
	}

	taskDesc, _ := planData["task_description"].(string)

	toolCallsRaw, ok := planData["tool_calls"].([]interface{})
	if !ok {
		return taskDesc, nil, fmt.Errorf("invalid tool calls in execution plan")
	}

	// Convert to ToolCall structs
	toolCalls, err := parseToolsNeeded(toolCallsRaw)
	if err != nil {
		return taskDesc, nil, fmt.Errorf("failed to parse tool calls: %v", err)
	}

	return taskDesc, toolCalls, nil
}

// ClearStoredToolCalls removes the stored tool calls
func ClearStoredToolCalls() {
	state := getTaskState()
	state.SetContext("has_execution_plan", false)
	state.SetContext("execution_plan", "")
}
