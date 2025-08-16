package tools

import (
	"testing"
)

func TestRegister(t *testing.T) {
	testFunc := func(args map[string]interface{}) (string, error) {
		return "test result", nil
	}

	// Test basic registration
	Register("test_tool", testFunc)

	if !containsString(List(), "test_tool") {
		t.Error("Basic registration failed")
	}

	// Test that tool is executable
	result, err := Execute("test_tool", map[string]interface{}{})
	if err != nil {
		t.Errorf("Failed to execute registered tool: %v", err)
	}
	if result != "test result" {
		t.Errorf("Expected result 'test result', got '%s'", result)
	}
}

func TestExecute(t *testing.T) {
	testFunc := func(args map[string]interface{}) (string, error) {
		return "executed", nil
	}

	Register("test_execute", testFunc)

	result, err := Execute("test_execute", map[string]interface{}{})
	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}
	if result != "executed" {
		t.Errorf("Expected 'executed', got '%s'", result)
	}
}

func TestExecuteNonExistent(t *testing.T) {
	_, err := Execute("non_existent_tool", map[string]interface{}{})
	if err == nil {
		t.Error("Expected error for non-existent tool")
	}
}

func TestList(t *testing.T) {
	// Clear registry for this test
	ClearRegistry()

	testFunc := func(args map[string]interface{}) (string, error) {
		return "", nil
	}

	Register("tool1", testFunc)
	Register("tool2", testFunc)

	tools := List()
	if len(tools) != 2 {
		t.Errorf("Expected 2 tools, got %d", len(tools))
	}

	if !containsString(tools, "tool1") {
		t.Error("tool1 not found in list")
	}
	if !containsString(tools, "tool2") {
		t.Error("tool2 not found in list")
	}
}

// Helper function to check if slice contains string
func containsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
