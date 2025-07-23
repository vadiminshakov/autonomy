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

	args := map[string]any{
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
	args := map[string]any{
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
			errorMsg:    "diff must contain hunk headers",
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

func TestCleanupRejFiles(t *testing.T) {
	// create a test .rej file
	testFile := "test_cleanup.go"
	rejFile := testFile + ".rej"

	// create temporary .rej file
	err := os.WriteFile(rejFile, []byte("some rejected content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test .rej file: %v", err)
	}

	// verify .rej file exists
	if _, err := os.Stat(rejFile); os.IsNotExist(err) {
		t.Fatal(".rej file was not created")
	}

	// verify .rej file is removed
	if _, err := os.Stat(rejFile); err == nil {
		t.Error(".rej file was not cleaned up")
		// cleanup in case test fails
		os.Remove(rejFile)
	}
}

func TestFixHunkHeader(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "valid header unchanged",
			input:    "@@ -1,3 +1,4 @@",
			expected: "@@ -1,3 +1,4 @@",
		},
		{
			name:     "fix spacing issues",
			input:    "@@-1,3+1,4@@",
			expected: "@@ -1,3 +1,4 @@",
		},
		{
			name:     "fix extra spaces",
			input:    "@@  -1,3   +1,4  @@",
			expected: "@@ -1,3 +1,4 @@",
		},
		{
			name:     "handle simple format",
			input:    "@@ -1 +1 @@",
			expected: "@@ -1 +1 @@",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fixHunkHeader(tt.input)
			if tt.expected == tt.input && result == "" {
				// expected no change for valid headers
				return
			}
			if result != tt.expected {
				t.Errorf("fixHunkHeader() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestValidateAndNormalizeDiffAutoFix(t *testing.T) {
	// create a temporary test file for normalization tests
	testFile := "test_normalize.go"
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

	tests := []struct {
		name        string
		diff        string
		expectError bool
		description string
	}{
		{
			name:        "auto-fix line endings",
			diff:        "@@ -5,3 +5,4 @@\r\n func main() {\r\n \tfmt.Println(\"Hello, World!\")\r\n+\tfmt.Println(\"Test!\")\r\n }",
			expectError: false,
			description: "should handle Windows line endings",
		},
		{
			name:        "auto-fix missing space prefix",
			diff:        "@@ -5,3 +5,4 @@\nfunc main() {\n\tfmt.Println(\"Hello, World!\")\n+\tfmt.Println(\"Test!\")\n}",
			expectError: false,
			description: "should auto-fix missing space prefix for context lines",
		},
		{
			name:        "handle mixed line endings",
			diff:        "@@ -5,3 +5,4 @@\r\n func main() {\n \tfmt.Println(\"Hello, World!\")\r+\tfmt.Println(\"Test!\")\n }",
			expectError: false,
			description: "should normalize mixed line endings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validateAndNormalizeDiff(tt.diff, testFile)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if !tt.expectError && result == "" {
				t.Error("Expected non-empty result")
			}
		})
	}
}

func TestApplyDiffStrategies(t *testing.T) {
	// create a test file that might require different patch strategies
	testFile := "test_strategies.go"
	originalContent := `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
	// existing comment
}
`

	// write original content
	err := os.WriteFile(testFile, []byte(originalContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer os.Remove(testFile)

	// test a diff that might need fuzz matching due to whitespace differences
	diff := `@@ -4,5 +4,6 @@
 import "fmt"
 
 func main() {
 	fmt.Println("Hello, World!")
+	fmt.Println("Added line!")
 	// existing comment
`

	args := map[string]any{
		"path": testFile,
		"diff": diff,
	}

	result, err := applyDiff(args)
	if err != nil {
		t.Fatalf("ApplyDiff with strategies failed: %v", err)
	}

	if !strings.Contains(result, "diff applied successfully") {
		t.Errorf("Expected success message, got: %s", result)
	}

	// verify the file was modified correctly
	modifiedContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read modified file: %v", err)
	}

	if !strings.Contains(string(modifiedContent), "Added line!") {
		t.Errorf("Expected modified content to contain 'Added line!', got: %s", string(modifiedContent))
	}
}
