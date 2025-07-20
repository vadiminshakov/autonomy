package entity

// Message represents a conversation entry
// It is used across the project to store chat history.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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
}

// AddMessage appends a new chat message.
func (p *PromptData) AddMessage(role, content string) {
	p.Messages = append(p.Messages, Message{Role: role, Content: content})
}

// forceToolsMessage is returned when the LLM ignored tool usage rules.
const forceToolsMessage = `ERROR: You must use tools!

You have access to many tools like get_project_structure, read_file, write_file, execute_command, etc.
Use the appropriate tool to accomplish the task.

NO TEXT. TOOLS ONLY.`

// GetForceToolsMessage returns a predefined message instructing the LLM to call tools.
func (p *PromptData) GetForceToolsMessage() string {
	return forceToolsMessage
}

// ToolCall describes a tool invocation requested by the LLM.
type ToolCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

// AIResponse is a generic response from an LLM provider containing either plain text or a list of tool invocations.
type AIResponse struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}
