package tools

import (
	"strings"
	"testing"
)

func TestAttemptCompletion_DirectTask(t *testing.T) {
	state := getTaskState()
	state.Reset()

	// test direct task completion
	args := map[string]interface{}{
		"result": "Task finished successfully",
	}

	result, err := AttemptCompletion(args)
	if err != nil {
		t.Fatalf("AttemptCompletion failed: %v", err)
	}

	if !strings.Contains(result, "Task completed:") {
		t.Errorf("Expected 'Task completed:' in result, got: %s", result)
	}

	if !strings.Contains(result, "Task finished successfully") {
		t.Errorf("Expected result to contain input message, got: %s", result)
	}

	// check that task_completed flag is set
	completed, exists := state.GetContext("task_completed")
	if !exists || completed != "true" {
		t.Errorf("Expected task_completed to be 'true', got: %v", completed)
	}
}

func TestAttemptCompletion_DecomposedTask(t *testing.T) {
	state := getTaskState()
	state.Reset()

	// set decomposed task flag
	state.SetContext("has_decomposed_task", true)

	args := map[string]interface{}{
		"result": "Step 1 completed",
	}

	result, err := AttemptCompletion(args)
	if err != nil {
		t.Fatalf("AttemptCompletion failed: %v", err)
	}

	if !strings.Contains(result, "Step completed:") {
		t.Errorf("Expected 'Step completed:' in result, got: %s", result)
	}

	if !strings.Contains(result, "Step 1 completed") {
		t.Errorf("Expected result to contain input message, got: %s", result)
	}

	// check that step_completed flag is set, not task_completed
	stepCompleted, exists := state.GetContext("step_completed")
	if !exists || stepCompleted != "true" {
		t.Errorf("Expected step_completed to be 'true', got: %v", stepCompleted)
	}

	// check that task_completed is not set
	taskCompleted, exists := state.GetContext("task_completed")
	if exists && taskCompleted == "true" {
		t.Errorf("Expected task_completed to not be set for decomposed task step")
	}
}

func TestAttemptCompletion_FailedLastTool(t *testing.T) {
	state := getTaskState()
	state.Reset()

	// simulate failed last tool
	state.RecordToolUse("some_tool", false, "error occurred")

	args := map[string]interface{}{
		"result": "Trying to complete",
	}

	_, err := AttemptCompletion(args)
	if err == nil {
		t.Errorf("Expected error when last tool failed, but got none")
	}

	if !strings.Contains(err.Error(), "cannot complete task: last operation failed") {
		t.Errorf("Expected specific error message, got: %v", err)
	}
}

func TestAttemptCompletion_NoResult(t *testing.T) {
	state := getTaskState()
	state.Reset()

	args := map[string]interface{}{}

	result, err := AttemptCompletion(args)
	if err != nil {
		t.Fatalf("AttemptCompletion failed: %v", err)
	}

	expected := "Task completed!\n\n✅"
	if result != expected {
		t.Errorf("Expected '%s', got: '%s'", expected, result)
	}
}

func TestAttemptCompletion_DecomposedTaskNoResult(t *testing.T) {
	state := getTaskState()
	state.Reset()

	// set decomposed task flag
	state.SetContext("has_decomposed_task", true)

	args := map[string]interface{}{}

	result, err := AttemptCompletion(args)
	if err != nil {
		t.Fatalf("AttemptCompletion failed: %v", err)
	}

	expected := "Step completed!\n✅"
	if result != expected {
		t.Errorf("Expected '%s', got: '%s'", expected, result)
	}
}