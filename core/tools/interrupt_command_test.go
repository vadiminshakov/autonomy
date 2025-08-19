package tools

import (
	"strings"
	"testing"
)

func TestInterruptCommand(t *testing.T) {
	result, err := InterruptCommand(map[string]interface{}{
		"command": "sleep 20",
	})

	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	if !strings.Contains(result, "command was interrupted after 10 seconds") {
		t.Errorf("expected interruption message, got: %s", result)
	}

	if !strings.Contains(result, "output analysis:") {
		t.Errorf("expected output analysis, got: %s", result)
	}
}

func TestInterruptCommandWithQuickCommand(t *testing.T) {
	result, err := InterruptCommand(map[string]interface{}{
		"command": "echo 'hello world'",
	})

	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	if !strings.Contains(result, "hello world") {
		t.Errorf("expected command output, got: %s", result)
	}
}

func TestInterruptCommandWithEmptyCommand(t *testing.T) {
	_, err := InterruptCommand(map[string]interface{}{
		"command": "",
	})

	if err == nil {
		t.Error("expected error for empty command")
	}

	if !strings.Contains(err.Error(), "parameter 'command' must be a non-empty string") {
		t.Errorf("expected specific error message, got: %v", err)
	}
}

func TestInterruptCommandWithDangerousCommand(t *testing.T) {
	_, err := InterruptCommand(map[string]interface{}{
		"command": "rm -rf /",
	})

	if err == nil {
		t.Error("expected error for dangerous command")
	}

	if !strings.Contains(err.Error(), "is blocked for security reasons") {
		t.Errorf("expected security error message, got: %v", err)
	}
}

func TestAnalyzeCommandOutput(t *testing.T) {
	output := "error: compilation failed\nwarning: unused variable\nbuilding...\nrunning tests..."
	command := "go run main.go"

	analysis := analyzeCommandOutput(output, command)

	if !strings.Contains(analysis, "errors detected in output") {
		t.Errorf("expected error detection, got: %s", analysis)
	}

	if !strings.Contains(analysis, "warnings detected in output") {
		t.Errorf("expected warning detection, got: %s", analysis)
	}

	if !strings.Contains(analysis, "command was in build/compilation phase") {
		t.Errorf("expected build phase detection, got: %s", analysis)
	}

	if !strings.Contains(analysis, "command was running tests") {
		t.Errorf("expected test detection, got: %s", analysis)
	}

	if !strings.Contains(analysis, "this appears to be a Go command") {
		t.Errorf("expected Go command detection, got: %s", analysis)
	}
}

func TestAnalyzeCommandOutputEmpty(t *testing.T) {
	analysis := analyzeCommandOutput("", "some command")

	if !strings.Contains(analysis, "no output captured before interruption") {
		t.Errorf("expected empty output message, got: %s", analysis)
	}
}

func TestInterruptCommandRecordsState(t *testing.T) {
	// reset task state
	if _, err := resetTaskState(map[string]interface{}{}); err != nil {
		t.Fatalf("failed to reset task state: %v", err)
	}

	// run interrupted command that produces output
	if _, err := InterruptCommand(map[string]interface{}{
		"command": "echo 'starting long process' && sleep 20",
	}); err != nil {
		t.Fatalf("InterruptCommand returned error: %v", err)
	}

	// check if state was recorded
	state := getTaskState()
	lastCmd, exists := state.GetContext("last_interrupted_command")
	if !exists {
		t.Error("expected last_interrupted_command to be recorded")
	}

	if lastCmd != "echo 'starting long process' && sleep 20" {
		t.Errorf("expected 'echo 'starting long process' && sleep 20', got: %v", lastCmd)
	}

	output, exists := state.GetContext("last_command_output")
	if !exists {
		t.Error("expected last_command_output to be recorded")
	}

	if outputStr, ok := output.(string); !ok {
		t.Error("expected last_command_output to be a string")
	} else if !strings.Contains(outputStr, "starting long process") {
		t.Errorf("expected command output to contain 'starting long process', got: %s", outputStr)
	}
}
