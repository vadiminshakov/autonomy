package entity

import (
	"fmt"
	"strings"
)

// Message represents a conversation entry
// It is used across the project to store chat history.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCallID string     `json:"tool_call_id,omitempty"` // for tool responses
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // for assistant messages with tools
}

// ToolDefinition describes a single tool available to the LLM agent.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
}

// PromptData aggregates all information required to build a prompt for the LLM.
type PromptData struct {
	SystemPrompt string
	Messages     []Message
	Tools        []ToolDefinition
	Model        string
}

// AddMessage appends a new chat message.
func (p *PromptData) AddMessage(role, content string) {
	p.Messages = append(p.Messages, Message{Role: role, Content: content})
}

// AddAssistantMessageWithTools appends an assistant message with tool calls.
func (p *PromptData) AddAssistantMessageWithTools(content string, toolCalls []ToolCall) {
	for i := range toolCalls {
		if toolCalls[i].ID == "" {
			// use toolu_ prefix for Anthropic Claude models, call_ for others
			if strings.Contains(strings.ToLower(p.Model), "claude") || strings.Contains(strings.ToLower(p.Model), "anthropic") {
				toolCalls[i].ID = fmt.Sprintf("toolu_%s_%d", toolCalls[i].Name, i)
			} else {
				toolCalls[i].ID = fmt.Sprintf("call_%d", i)
			}
		}
	}
	p.Messages = append(p.Messages, Message{
		Role:      "assistant",
		Content:   content,
		ToolCalls: toolCalls,
	})
}

// AddToolResponse appends a tool response message.
func (p *PromptData) AddToolResponse(toolCallID, result string) {
	p.Messages = append(p.Messages, Message{
		Role:       "tool",
		Content:    result,
		ToolCallID: toolCallID,
	})
}

// forceToolsMessage is returned when the LLM ignored tool usage rules.
const forceToolsMessage = `You must use a tool to help with this task.

Available tools:
- get_project_structure: explore the project layout
- view: read a specific file with line numbers
- write: create or modify files
- multiedit: make multiple edits to a single file
- grep: universal search for patterns in files and filenames
- bash: run shell commands
- diagnostics: run code analysis and linting
- attempt_completion: finish the task

Choose the most appropriate tool for what you need to do and execute it now.`

// GetForceToolsMessage returns a predefined message instructing the LLM to call tools.
func (p *PromptData) GetForceToolsMessage() string {
	return forceToolsMessage
}

// ToolCall describes a tool invocation requested by the LLM.
type ToolCall struct {
	ID   string         `json:"id,omitempty"`
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

// AIResponse is a generic response from an LLM provider containing either plain text or a list of tool invocations.
type AIResponse struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}
