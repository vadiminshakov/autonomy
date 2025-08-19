package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/vadiminshakov/autonomy/core/config"
	"github.com/vadiminshakov/autonomy/core/decomposition"
)

func init() {
	Register("decompose_task", DecomposeTask)
}

// DecomposeTask breaks down a complex task into executable steps using AI
func DecomposeTask(args map[string]interface{}) (string, error) {
	taskDesc, ok := args["task_description"].(string)
	if !ok || taskDesc == "" {
		return "", fmt.Errorf("parameter 'task_description' must be a non-empty string")
	}

	// Check if we already have a decomposed task to prevent repeated planning
	state := getTaskState()
	if hasTask, exists := state.GetContext("has_decomposed_task"); exists && hasTask == true {
		return "", fmt.Errorf("task already decomposed - use existing plan or clear first")
	}

	cfg, err := config.LoadConfigFile()
	if err != nil {
		return "", fmt.Errorf("failed to load config: %v", err)
	}

	decomposer, err := decomposition.NewTaskDecomposer(cfg)
	if err != nil {
		return "", fmt.Errorf("failed to create task decomposer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	result, err := decomposer.DecomposeTask(ctx, taskDesc)
	if err != nil {
		return "", fmt.Errorf("failed to decompose task: %v", err)
	}

	taskState := getTaskState()
	taskState.SetContext("has_decomposed_task", true)
	taskState.SetContext("decomposed_task", result)
	taskState.SetContext("has_execution_plan", true)

	planData := map[string]any{
		"task_description": taskDesc,
		"decomposition":    result,
	}

	planJSON, err := json.Marshal(planData)
	if err != nil {
		return "", fmt.Errorf("failed to serialize plan: %v", err)
	}

	taskState.SetContext("execution_plan", string(planJSON))

	summary := result.GetStepSummary()
	return summary, nil
}

// HasDecomposedTask checks if there's a decomposed task available
func HasDecomposedTask() bool {
	state := getTaskState()
	hasTask, exists := state.GetContext("has_decomposed_task")
	return exists && hasTask == true
}

// GetDecomposedTask retrieves the stored decomposed task
func GetDecomposedTask() (*decomposition.DecompositionResult, error) {
	state := getTaskState()

	hasTask, exists := state.GetContext("has_decomposed_task")
	if !exists || hasTask != true {
		return nil, fmt.Errorf("no decomposed task found")
	}

	taskData, exists := state.GetContext("decomposed_task")
	if !exists {
		return nil, fmt.Errorf("decomposed task flag set but no task data found")
	}

	result, ok := taskData.(*decomposition.DecompositionResult)
	if !ok {
		return nil, fmt.Errorf("invalid decomposed task data type")
	}

	return result, nil
}

// ClearDecomposedTask removes the stored decomposed task
func ClearDecomposedTask() {
	state := getTaskState()
	state.SetContext("has_decomposed_task", false)
	state.SetContext("decomposed_task", nil)
}
