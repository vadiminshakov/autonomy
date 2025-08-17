package tools

import (
	"strings"
	"testing"
	"time"
)

func TestGetTaskSummary(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]interface{}
		expected []string
	}{
		{
			name: "Basic summary",
			args: map[string]interface{}{
				"duration":    2 * time.Minute,
				"total_steps": 5,
				"errors":      []string{"go_vet: exit status 1", "timeout error"},
			},
			expected: []string{
				"# Task Execution Summary",
				"## Status",
				"Task execution completed",
				"Execution time: 2m0s",
				"Total steps executed: 5",
				"## Errors",
				"go_vet: exit status 1",
				"timeout error",
			},
		},
		{
			name: "Summary without errors",
			args: map[string]interface{}{
				"duration":    30 * time.Second,
				"total_steps": 3,
			},
			expected: []string{
				"# Task Execution Summary",
				"## Status",
				"Task execution completed",
				"Execution time: 30s",
				"Total steps executed: 3",
			},
		},
		{
			name: "Empty summary",
			args: map[string]interface{}{},
			expected: []string{
				"# Task Execution Summary",
				"## Status",
				"Task execution completed",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetDetailedTaskSummary(tt.args)
			if err != nil {
				t.Errorf("GetDetailedTaskSummary() error = %v", err)
				return
			}

			for _, expected := range tt.expected {
				if !strings.Contains(result, expected) {
					t.Errorf("GetDetailedTaskSummary() result missing expected string: %q\nGot: %s", expected, result)
				}
			}
		})
	}
}

func TestAttemptCompletionWithSummary(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]interface{}
		expected []string
	}{
		{
			name: "Completion with summary",
			args: map[string]interface{}{
				"result": "analysis completed",
			},
			expected: []string{
				"🎉 Task completed:",
				"analysis completed",
			},
		},
		{
			name: "Completion without summary",
			args: map[string]interface{}{
				"result": "simple completion",
			},
			expected: []string{
				"🎉 Task completed:",
				"simple completion",
			},
		},
		{
			name: "Empty completion with summary",
			args: map[string]interface{}{
				"result": "Task completed successfully",
			},
			expected: []string{
				"🎉 Task completed",
				"Task completed successfully",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := AttemptCompletion(tt.args)
			if err != nil {
				t.Errorf("AttemptCompletion() error = %v", err)
				return
			}

			for _, expected := range tt.expected {
				if !strings.Contains(result, expected) {
					t.Errorf("AttemptCompletion() result missing expected string: %q\nGot: %s", expected, result)
				}
			}
		})
	}
}

func TestFormatTaskSummary(t *testing.T) {
	summary := &TaskSummaryData{
		TotalSteps:     10,
		CompletedSteps: 8,
		FailedSteps:    2,
		ExecutionTime:  5 * time.Minute,
		FilesModified:  []string{"file1.go", "file2.go"},
		FilesRead:      []string{"config.json", "main.go"},
		KeyFindings:    []string{"High complexity detected", "Tests passed"},
		Errors:         []string{"Timeout in search", "Compilation error"},
	}

	result := formatTaskSummary(summary)

	expectedStrings := []string{
		"# Task Execution Summary",
		"## Statistics",
		"**Total Steps**: 10",
		"**Completed Steps**: 8",
		"**Failed Steps**: 2",
		"**Execution Time**: 5m0s",
		"## Files Modified",
		"file1.go",
		"file2.go",
		"## Files Read",
		"config.json",
		"main.go",
		"## Key Findings",
		"High complexity detected",
		"Tests passed",
		"## Errors",
		"Timeout in search",
		"Compilation error",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(result, expected) {
			t.Errorf("formatTaskSummary() result missing expected string: %q\nGot: %s", expected, result)
		}
	}
}
