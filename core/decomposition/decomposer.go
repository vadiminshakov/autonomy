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

type TaskStep struct {
	ID           string   `json:"id"`
	Description  string   `json:"description"`
	Reason       string   `json:"reason"`
	Dependencies []string `json:"dependencies,omitempty"`
	Status       string   `json:"status,omitempty"` // pending, in_progress, completed, failed
	MaxAttempts  int      `json:"max_attempts,omitempty"` // maximum number of attempts
}

type DecompositionResult struct {
	OriginalTask string     `json:"original_task"`
	Steps        []TaskStep `json:"steps"`
	Reasoning    string     `json:"reasoning"`
}

type TaskDecomposer struct {
	aiClient ai.AIClient
}

func NewTaskDecomposer(cfg config.Config) (*TaskDecomposer, error) {
	client, err := ai.ProvideAiClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create AI client: %v", err)
	}

	return &TaskDecomposer{
		aiClient: client,
	}, nil
}

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

	result, err := td.parseDecompositionResponse(response.Content, taskDescription)
	if err != nil {
		return nil, fmt.Errorf("failed to parse decomposition response: %v", err)
	}

	return result, nil
}

func (td *TaskDecomposer) buildDecompositionPrompt(taskDescription string, availableTools []entity.ToolDefinition) entity.PromptData {
	systemPrompt := `You are a task decomposition expert. Follow this structured approach for breaking down programming tasks:

1. ANALYZE: Understand the task requirements, current state, and desired outcome
2. PLAN: Design a minimal, logical sequence of steps
3. STRUCTURE: Organize steps with clear dependencies and objectives

CRITICAL DECOMPOSITION RULES:
1. Keep decomposition MINIMAL - aim for 3-6 steps maximum for most tasks
2. Each step should describe WHAT needs to be done, not HOW (no specific tools mentioned)
3. Steps should be logically necessary, not just "nice to have"
4. Avoid redundant verification/checking steps unless critical
5. Group related operations when possible
6. Focus on the core deliverable, not every possible side task
7. Each step should be substantial enough to require AI decision-making

CONTEXT AWARENESS:
- You have access to full file contents (not truncated)
- Project structure and dependencies are available
- Previous task context and history are preserved
- Language server information provides semantic understanding

TASK COMPLEXITY GUIDELINES:
- Simple tasks (create single file, read something): 2-3 steps maximum
- Medium tasks (implement feature): 3-5 steps maximum  
- Complex tasks (refactor system): 4-7 steps maximum

STEP COMPLETION CRITERIA:
Each step should have clear success criteria:
- The specific objective is achieved
- No errors remain unresolved
- Implementation follows best practices
- Tests pass (if applicable)

TOOL USAGE EFFICIENCY:
- Read files completely before making changes
- Avoid redundant operations
- Use appropriate tools for each task type
- Combine related operations when possible

GOOD STEP EXAMPLES:
- "Analyze the existing code structure to understand current implementation"
- "Implement the new authentication middleware with proper error handling"
- "Update all existing route handlers to use the new middleware"
- "Create comprehensive tests for the authentication flow"

BAD STEP EXAMPLES:
- "Use read_file to check main.go" (too specific about tools)
- "Verify directory exists" (unnecessary when other tools handle this)
- "Run tests" (unless testing is the main goal)

OUTPUT FORMAT:
Provide your response as a JSON object with this structure:
{
  "reasoning": "Explanation of your decomposition approach and why these steps are necessary",
  "steps": [
    {
      "id": "step_1",
      "description": "Clear description of what needs to be accomplished in this step",
      "reason": "Why this step is necessary for the overall task",
      "dependencies": ["step_id"] // optional, only if this step depends on completion of other steps
    }
  ]
}

IMPORTANT: Your response must be valid JSON only, no additional text.`

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

	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimSpace(content)
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSpace(content)
	}

	if strings.HasSuffix(content, "```") {
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	}

	var rawResult struct {
		Reasoning string     `json:"reasoning"`
		Steps     []TaskStep `json:"steps"`
	}

	if err := json.Unmarshal([]byte(content), &rawResult); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %v\nContent: %s", err, content)
	}

	for i := range rawResult.Steps {
		step := &rawResult.Steps[i]

		if step.ID == "" {
			step.ID = fmt.Sprintf("step_%d", i+1)
		}

		if step.Description == "" {
			return nil, fmt.Errorf("step %s missing description", step.ID)
		}

		// initialize status and attempts
		if step.Status == "" {
			step.Status = "pending"
		}
		if step.MaxAttempts == 0 {
			step.MaxAttempts = 3 // default 3 attempts
		}
	}

	return &DecompositionResult{
		OriginalTask: originalTask,
		Steps:        rawResult.Steps,
		Reasoning:    rawResult.Reasoning,
	}, nil
}

func (dr *DecompositionResult) GetStepSummary() string {
	var summary strings.Builder

	summary.WriteString(fmt.Sprintf("Task Decomposition: %s\n\n", dr.OriginalTask))

	if dr.Reasoning != "" {
		summary.WriteString(fmt.Sprintf("Approach: %s\n\n", dr.Reasoning))
	}

	summary.WriteString(fmt.Sprintf("Execution Plan (%d steps):\n", len(dr.Steps)))

	for i, step := range dr.Steps {
		summary.WriteString(fmt.Sprintf("  %d. %s\n", i+1, step.Description))

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
