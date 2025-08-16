package entity

import "encoding/json"

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
}

type PromptData struct {
	SystemPrompt string
	Messages     []Message
	Tools        []ToolDefinition
}

func (p *PromptData) AddMessage(role, content string) {
	p.Messages = append(p.Messages, Message{Role: role, Content: content})
}

func (p *PromptData) AddAssistantMessageWithTools(content string, toolCalls []ToolCall) {
	p.Messages = append(p.Messages, Message{
		Role:      "assistant",
		Content:   content,
		ToolCalls: toolCalls,
	})
}

func (p *PromptData) AddToolResponse(toolCallID, result string) {
	p.Messages = append(p.Messages, Message{
		Role:       "tool",
		Content:    result,
		ToolCallID: toolCallID,
	})
}

const forceToolsMessage = `You must use a tool to help with this task.

Available tools:
- get_project_structure: explore the project layout
- read_file: read a specific file with line numbers
- write_file: create or modify files
- apply_diff: apply changes to existing files
- search_dir: search for patterns in files
- find_files: find files by name/pattern
- execute_command: run shell commands
- go_test: run Go tests
- go_vet: run Go code analysis
- attempt_completion: finish the task

Choose the most appropriate tool for what you need to do and execute it now.`

func (p *PromptData) GetForceToolsMessage() string {
	return forceToolsMessage
}

type ToolCall struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Function  FunctionCall   `json:"function"`
	Arguments string         `json:"arguments,omitempty"`
	Name      string         `json:"name,omitempty"`
	Args      map[string]any `json:"args,omitempty"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

func NewToolCall(id, toolType string, function FunctionCall) ToolCall {
	tc := ToolCall{
		ID:       id,
		Type:     toolType,
		Function: function,
		Name:     function.Name,
	}

	if function.Arguments != "" {
		var args map[string]any
		if err := json.Unmarshal([]byte(function.Arguments), &args); err == nil {
			tc.Args = args
		} else {
			tc.Args = make(map[string]any)
		}
	} else {
		tc.Args = make(map[string]any)
	}

	return tc
}

type AIResponse struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}
