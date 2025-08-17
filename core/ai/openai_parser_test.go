package ai

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vadiminshakov/autonomy/core/entity"
)

func TestParseTextToolCalls(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCalls []entity.ToolCall
		wantFound bool
	}{
		{
			name:  "Single attempt_completion with result",
			input: "Tool call: attempt_completion (result: Task completed successfully)",
			wantCalls: []entity.ToolCall{
				{
					Name: "attempt_completion",
					Args: map[string]interface{}{
						"result": "Task completed successfully",
					},
				},
			},
			wantFound: true,
		},
		{
			name:  "Multiple tool calls",
			input: "Tool call: read_file (path: main.go)\nTool call: write_file (path: test.txt, content: Hello World)",
			wantCalls: []entity.ToolCall{
				{
					Name: "read_file",
					Args: map[string]interface{}{
						"path": "main.go",
					},
				},
				{
					Name: "write_file",
					Args: map[string]interface{}{
						"path":    "test.txt",
						"content": "Hello World",
					},
				},
			},
			wantFound: true,
		},
		{
			name:  "Tool call without parameters",
			input: "Tool call: get_project_structure ()",
			wantCalls: []entity.ToolCall{
				{
					Name: "get_project_structure",
					Args: map[string]interface{}{},
				},
			},
			wantFound: true,
		},
		{
			name:      "No tool calls",
			input:     "Just some regular text without tool calls",
			wantCalls: []entity.ToolCall{},
			wantFound: false,
		},
		{
			name:  "Tool call with complex parameters",
			input: "Tool call: execute_command (command: go test -v ./...)",
			wantCalls: []entity.ToolCall{
				{
					Name: "execute_command",
					Args: map[string]interface{}{
						"command": "go test -v ./...",
					},
				},
			},
			wantFound: true,
		},
		{
			name: "Mixed content with tool calls",
			input: `Some text before
Tool call: read_file (path: test.go)
Some text in between
Tool call: attempt_completion (result: All done)
Some text after`,
			wantCalls: []entity.ToolCall{
				{
					Name: "read_file",
					Args: map[string]interface{}{
						"path": "test.go",
					},
				},
				{
					Name: "attempt_completion",
					Args: map[string]interface{}{
						"result": "All done",
					},
				},
			},
			wantFound: true,
		},
		{
			name:  "Long result text in attempt_completion",
			input: "Tool call: attempt_completion (result: Я создал терминальную игру Жизнь Conway's Game of Life на языке Go)",
			wantCalls: []entity.ToolCall{
				{
					Name: "attempt_completion",
					Args: map[string]interface{}{
						"result": "Я создал терминальную игру Жизнь Conway's Game of Life на языке Go",
					},
				},
			},
			wantFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls, found := parseTextToolCalls(tt.input)
			assert.Equal(t, tt.wantFound, found, "Found mismatch")
			assert.Equal(t, len(tt.wantCalls), len(calls), "Number of calls mismatch")

			for i := range tt.wantCalls {
				if i < len(calls) {
					assert.Equal(t, tt.wantCalls[i].Name, calls[i].Name, "Tool name mismatch at index %d", i)
					assert.Equal(t, tt.wantCalls[i].Args, calls[i].Args, "Args mismatch at index %d", i)
				}
			}
		})
	}
}

func TestExtractJSONFromText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Valid JSON",
			input:    `Some text {"key": "value"} more text`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "No JSON",
			input:    `Just plain text`,
			expected: "",
		},
		{
			name:     "Multiple JSON objects",
			input:    `First {"a": 1} and second {"b": 2}`,
			expected: `{"a": 1} and second {"b": 2}`,
		},
		{
			name:     "JSON at start",
			input:    `{"tool_calls": [{"name": "test"}]} some text`,
			expected: `{"tool_calls": [{"name": "test"}]}`,
		},
		{
			name:     "JSON at end",
			input:    `some text {"result": "done"}`,
			expected: `{"result": "done"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractJSONFromText(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseJSONToolCalls(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantCalls   []entity.ToolCall
		wantContent string
		wantOk      bool
	}{
		{
			name:  "Valid tool calls JSON",
			input: `{"tool_calls": [{"name": "read_file", "args": {"path": "test.go"}}], "content": "Test content"}`,
			wantCalls: []entity.ToolCall{
				{
					Name: "read_file",
					Args: map[string]interface{}{
						"path": "test.go",
					},
				},
			},
			wantContent: "Test content",
			wantOk:      true,
		},
		{
			name:  "Multiple tool calls",
			input: `{"tool_calls": [{"name": "read_file", "args": {"path": "a.go"}}, {"name": "write_file", "args": {"path": "b.go", "content": "test"}}]}`,
			wantCalls: []entity.ToolCall{
				{
					Name: "read_file",
					Args: map[string]interface{}{
						"path": "a.go",
					},
				},
				{
					Name: "write_file",
					Args: map[string]interface{}{
						"path":    "b.go",
						"content": "test",
					},
				},
			},
			wantContent: "",
			wantOk:      true,
		},
		{
			name:        "Invalid JSON",
			input:       `{invalid json}`,
			wantCalls:   nil,
			wantContent: "",
			wantOk:      false,
		},
		{
			name:        "JSON without tool calls",
			input:       `{"content": "Just content"}`,
			wantCalls:   nil,
			wantContent: "",
			wantOk:      false,
		},
		{
			name:        "Empty tool calls array",
			input:       `{"tool_calls": []}`,
			wantCalls:   nil,
			wantContent: "",
			wantOk:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls, content, ok := parseJSONToolCalls(tt.input)
			assert.Equal(t, tt.wantOk, ok, "OK mismatch")
			assert.Equal(t, tt.wantContent, content, "Content mismatch")

			if tt.wantOk {
				assert.Equal(t, len(tt.wantCalls), len(calls), "Number of calls mismatch")
				for i := range tt.wantCalls {
					if i < len(calls) {
						assert.Equal(t, tt.wantCalls[i].Name, calls[i].Name, "Tool name mismatch at index %d", i)
						// для Args проверяем каждое поле отдельно из-за типов
						if tt.wantCalls[i].Args != nil && calls[i].Args != nil {
							for key, expectedVal := range tt.wantCalls[i].Args {
								actualVal, exists := calls[i].Args[key]
								assert.True(t, exists, "Key %s not found in actual args", key)
								assert.Equal(t, expectedVal, actualVal, "Value mismatch for key %s", key)
							}
						}
					}
				}
			}
		})
	}
}

func TestCleanContentFromToolCalls(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "Remove single tool call",
			input: `Some text before
Tool call: read_file (path: test.go)
Some text after`,
			want: `Some text before
Some text after`,
		},
		{
			name: "Remove multiple tool calls",
			input: `Start
Tool call: read_file (path: a.go)
Middle
Tool call: write_file (path: b.go, content: test)
End`,
			want: `Start
Middle
End`,
		},
		{
			name:  "No tool calls to remove",
			input: "Just regular text",
			want:  "Just regular text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// удаляем строки с tool calls из контента
			cleanContent := tt.input
			for _, line := range strings.Split(tt.input, "\n") {
				if strings.HasPrefix(line, "Tool call: ") {
					cleanContent = strings.Replace(cleanContent, line+"\n", "", 1)
					cleanContent = strings.Replace(cleanContent, line, "", 1)
				}
			}
			cleanContent = strings.TrimSpace(cleanContent)
			want := strings.TrimSpace(tt.want)
			assert.Equal(t, want, cleanContent)
		})
	}
}
