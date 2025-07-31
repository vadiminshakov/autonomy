package decomposition

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vadiminshakov/autonomy/core/ai"
	"github.com/vadiminshakov/autonomy/core/config"
	"github.com/vadiminshakov/autonomy/core/entity"
)

// AIClient interface for generating AI responses
type AIClient interface {
	GenerateCode(ctx context.Context, promptData entity.PromptData) (*entity.AIResponse, error)
}

// TaskStep represents a single step in task decomposition
type TaskStep struct {
	ID           string                 `json:"id"`
	Description  string                 `json:"description"`
	Tool         string                 `json:"tool"`
	Args         map[string]interface{} `json:"args"`
	Reason       string                 `json:"reason"`
	Dependencies []string               `json:"dependencies,omitempty"`
}

// DecompositionResult contains the result of task decomposition
type DecompositionResult struct {
	OriginalTask string     `json:"original_task"`
	Steps        []TaskStep `json:"steps"`
	Reasoning    string     `json:"reasoning"`
}

// TaskDecomposer breaks down complex tasks into executable steps
type TaskDecomposer struct {
	aiClient AIClient
}

// NewTaskDecomposer creates a new task decomposer
func NewTaskDecomposer(cfg config.Config) (*TaskDecomposer, error) {
	var client AIClient
	var err error

	switch cfg.Provider {
	case "anthropic":
		client, err = ai.NewAnthropic(cfg)
	case "openai", "openrouter":
		client, err = ai.NewOpenai(cfg)
	default:
		client, err = ai.NewOpenai(cfg) // default to OpenAI
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create AI client: %v", err)
	}

	return &TaskDecomposer{
		aiClient: client,
	}, nil
}

// DecomposeTask breaks down a complex task into executable steps
func (td *TaskDecomposer) DecomposeTask(
	ctx context.Context,
	taskDescription string,
	availableTools []entity.ToolDefinition,
) (*DecompositionResult, error) {
	prompt := td.buildDecompositionPrompt(taskDescription, availableTools)

	response, err := td.aiClient.GenerateCode(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to get AI response: %v", err)
	}

	// parse the response to extract steps
	result, err := td.parseDecompositionResponse(response.Content, taskDescription)
	if err != nil {
		return nil, fmt.Errorf("failed to parse decomposition response: %v", err)
	}

	return result, nil
}

// buildToolsList creates a formatted list of available tools from ToolDefinitions
func (td *TaskDecomposer) buildToolsList(tools []entity.ToolDefinition) string {
	var toolsList strings.Builder

	for _, tool := range tools {
		toolsList.WriteString(fmt.Sprintf("- %s: %s, args: %v\n", tool.Name, tool.Description, tool.InputSchema))
	}

	return toolsList.String()
}

// buildDecompositionPrompt creates a prompt for task decomposition
//
//nolint:lll
func (td *TaskDecomposer) buildDecompositionPrompt(taskDescription string, availableTools []entity.ToolDefinition) entity.PromptData {
	toolsList := td.buildToolsList(availableTools)
	systemPrompt := fmt.Sprintf(`You are a task decomposition expert. Your job is to break down complex programming tasks into specific, executable steps.

Available tools that can be used in steps:
%s

DECOMPOSITION RULES:
1. Break the task into logical, sequential steps
2. Each step should use exactly ONE tool
3. Steps should be atomic and focused
4. Include dependencies between steps when needed
5. Provide clear reasoning for each step
6. Start with analysis/understanding steps before making changes
7. End with validation/testing steps when appropriate
8. Avoid over-decomposition - keep steps meaningful and substantial

OUTPUT FORMAT:
Provide your response as a JSON object with this structure:
{
  "reasoning": "Explanation of your decomposition approach",
  "steps": [
    {
      "id": "step_1",
      "description": "Clear description of what this step does",
      "tool": "tool_name",
      "args": {"param": "value"},
      "reason": "Why this step is needed",
      "dependencies": ["step_id"] // optional, only if depends on other steps
    }
  ]
}

IMPORTANT: Your response must be valid JSON only, no additional text.`, toolsList)

	userMessage := fmt.Sprintf("Break down this task into executable steps: %s", taskDescription)

	return entity.PromptData{
		SystemPrompt: systemPrompt,
		Messages: []entity.Message{
			{Role: "user", Content: userMessage},
		},
		Tools: []entity.ToolDefinition{},
	}
}

// parseDecompositionResponse parses the AI response into a structured result
func (td *TaskDecomposer) parseDecompositionResponse(content, originalTask string) (*DecompositionResult, error) {
	// Clean the response - remove any markdown formatting
	content = strings.TrimSpace(content)

	// Remove markdown code blocks
	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimSpace(content)
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSpace(content)
	}

	// Remove trailing code block markers
	if strings.HasSuffix(content, "```") {
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	}

	// Parse JSON response
	var rawResult struct {
		Reasoning string     `json:"reasoning"`
		Steps     []TaskStep `json:"steps"`
	}

	if err := json.Unmarshal([]byte(content), &rawResult); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %v\nContent: %s", err, content)
	}

	for i := range rawResult.Steps {
		step := &rawResult.Steps[i]

		// generate ID if missing
		if step.ID == "" {
			step.ID = fmt.Sprintf("step_%d", i+1)
		}

		// validate required fields
		if step.Description == "" {
			return nil, fmt.Errorf("step %s missing description", step.ID)
		}
		if step.Tool == "" {
			return nil, fmt.Errorf("step %s missing tool", step.ID)
		}
		if step.Args == nil {
			step.Args = make(map[string]interface{})
		}
	}

	return &DecompositionResult{
		OriginalTask: originalTask,
		Steps:        rawResult.Steps,
		Reasoning:    rawResult.Reasoning,
	}, nil
}

// ConvertToToolCalls converts decomposition steps to tool calls
func (dr *DecompositionResult) ConvertToToolCalls() []entity.ToolCall {
	toolCalls := make([]entity.ToolCall, len(dr.Steps))

	for i, step := range dr.Steps {
		toolCalls[i] = entity.ToolCall{
			Name: step.Tool,
			Args: step.Args,
		}
	}

	return toolCalls
}

// GetStepSummary returns a human-readable summary of the decomposition
func (dr *DecompositionResult) GetStepSummary() string {
	var summary strings.Builder

	summary.WriteString(fmt.Sprintf("Task Decomposition: %s\n\n", dr.OriginalTask))

	if dr.Reasoning != "" {
		summary.WriteString(fmt.Sprintf("Approach: %s\n\n", dr.Reasoning))
	}

	summary.WriteString(fmt.Sprintf("Execution Plan (%d steps):\n", len(dr.Steps)))

	for i, step := range dr.Steps {
		summary.WriteString(fmt.Sprintf("  %d. %s\n", i+1, step.Description))
		summary.WriteString(fmt.Sprintf("     Tool: %s\n", step.Tool))

		if step.Reason != "" {
			summary.WriteString(fmt.Sprintf("     Reason: %s\n", step.Reason))
		}

		if len(step.Dependencies) > 0 {
			summary.WriteString(fmt.Sprintf("     Dependencies: %v\n", step.Dependencies))
		}

		summary.WriteString("\n")
	}

	return summary.String()
}
