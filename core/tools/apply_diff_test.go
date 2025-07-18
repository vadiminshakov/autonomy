package tools

import (
	"os"
	"strings"
	"testing"
)

func TestApplyDiff_ValidDiff(t *testing.T) {
	// create a temporary test file
	testFile := "test_apply_diff.go"
	originalContent := `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}
`

	// write original content
	err := os.WriteFile(testFile, []byte(originalContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer os.Remove(testFile)

	// create a valid unified diff
	diff := "@@ -5,3 +5,4 @@\n func main() {\n \tfmt.Println(\"Hello, World!\")\n+\tfmt.Println(\"This is a test!\")\n }"

	args := map[string]interface{}{
		"path": testFile,
		"diff": diff,
	}

	result, err := applyDiff(args)
	if err != nil {
		t.Fatalf("ApplyDiff failed: %v", err)
	}

	if !strings.Contains(result, "diff applied successfully") {
		t.Errorf("Expected success message, got: %s", result)
	}

	// verify the file was modified
	modifiedContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read modified file: %v", err)
	}

	if !strings.Contains(string(modifiedContent), "This is a test!") {
		t.Errorf("Expected modified content to contain 'This is a test!', got: %s", string(modifiedContent))
	}
}

func TestApplyDiff_InvalidDiff(t *testing.T) {
	args := map[string]interface{}{
		"path": "nonexistent.go",
		"diff": "invalid diff content",
	}

	_, err := applyDiff(args)
	if err == nil {
		t.Error("Expected error for invalid diff, but got none")
	}

	if !strings.Contains(err.Error(), "target file does not exist") {
		t.Errorf("Expected 'target file does not exist' error, got: %v", err)
	}
}

func TestApplyDiff_MissingParameters(t *testing.T) {
	tests := []struct {
		name          string
		args          map[string]interface{}
		expectedError string
	}{
		{
			name: "missing diff",
			args: map[string]interface{}{
				"path": "test.go",
			},
			expectedError: "parameter 'diff' must be a non-empty string",
		},
		{
			name: "missing path",
			args: map[string]interface{}{
				"diff": "@@ -1,1 +1,2 @@\n test\n+added",
			},
			expectedError: "either 'path' or 'file' parameter must be provided",
		},
		{
			name: "empty diff",
			args: map[string]interface{}{
				"path": "test.go",
				"diff": "",
			},
			expectedError: "parameter 'diff' must be a non-empty string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := applyDiff(tt.args)
			if err == nil {
				t.Error("Expected error but got none")
			}
			if !strings.Contains(err.Error(), tt.expectedError) {
				t.Errorf("Expected error containing '%s', got: %v", tt.expectedError, err)
			}
		})
	}
}

func TestValidateAndNormalizeDiff(t *testing.T) {
	tests := []struct {
		name        string
		diff        string
		filePath    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid diff with headers",
			diff:        "--- test.go\n+++ test.go\n@@ -1,3 +1,4 @@\n package main\n \n func main() {\n+\tfmt.Println(\"test\")\n }",
			filePath:    "test.go",
			expectError: false,
		},
		{
			name:        "valid diff without headers",
			diff:        "@@ -1,3 +1,4 @@\n package main\n \n func main() {\n+\tfmt.Println(\"test\")\n }",
			filePath:    "test.go",
			expectError: false,
		},
		{
			name:        "invalid diff without hunk headers",
			diff:        "package main\n+\tfmt.Println(\"test\")",
			filePath:    "test.go",
			expectError: true,
			errorMsg:    "diff must contain hunk headers (@@)",
		},
		{
			name:        "invalid hunk header format",
			diff:        "@@ invalid @@\n package main\n+\tfmt.Println(\"test\")",
			filePath:    "test.go",
			expectError: true,
			errorMsg:    "invalid hunk header format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validateAndNormalizeDiff(tt.diff, tt.filePath)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing '%s', got: %v", tt.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result == "" {
					t.Error("Expected non-empty result")
				}
			}
		})
	}
}
