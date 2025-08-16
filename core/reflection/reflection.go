package reflection

import (
	"context"
	"fmt"
	"strings"

	"github.com/vadiminshakov/autonomy/core/ai"
	"github.com/vadiminshakov/autonomy/core/entity"
	"github.com/vadiminshakov/autonomy/core/types"
)

type ReflectionResult struct {
	TaskCompleted bool   `json:"task_completed"`
	Reason        string `json:"reason"`
	ShouldRetry   bool   `json:"should_retry"`
}

type ReflectionEngine struct {
	aiClient ai.AIClient
}

func NewReflectionEngine(aiClient ai.AIClient) *ReflectionEngine {
	return &ReflectionEngine{
		aiClient: aiClient,
	}
}

func (sre *ReflectionEngine) EvaluateCompletion(
	ctx context.Context,
	plan *types.ExecutionPlan,
	originalTask string,
) (*ReflectionResult, error) {
	promptData := sre.createEvaluationPrompt(plan, originalTask)

	response, err := sre.aiClient.GenerateCode(ctx, promptData)
	if err != nil {
		return sre.simpleEvaluation(plan), nil
	}

	return sre.parseResponse(response.Content), nil
}

func (sre *ReflectionEngine) createEvaluationPrompt(plan *types.ExecutionPlan, originalTask string) entity.PromptData {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("ORIGINAL TASK: %s\n\n", originalTask))
	prompt.WriteString("EXECUTION RESULTS:\n")

	plan.RLock()
	successCount := 0
	totalSteps := len(plan.Steps)

	for i, step := range plan.Steps {
		status := string(step.Status)
		prompt.WriteString(fmt.Sprintf("%d. %s - %s", i+1, step.ToolName, status))

		if step.Error != nil {
			prompt.WriteString(fmt.Sprintf(" (Error: %s)", step.Error.Error()))
		}

		if step.Status == types.StepStatusCompleted {
			successCount++
		}

		prompt.WriteString("\n")
	}
	plan.RUnlock()

	prompt.WriteString(fmt.Sprintf("\nSUCCESS RATE: %d/%d steps completed\n", successCount, totalSteps))

	prompt.WriteString(`
Please evaluate if the original task was completed successfully.

Answer in this format:
COMPLETED: yes/no
REASON: brief explanation
RETRY: yes/no (if task should be retried)

Consider:
- Did we achieve the original goal?
- Are there critical errors that prevent completion?
- Is the task in a good final state?
`)

	return entity.PromptData{
		Messages: []entity.Message{
			{Role: "user", Content: prompt.String()},
		},
	}
}

func (sre *ReflectionEngine) parseResponse(response string) *ReflectionResult {
	result := &ReflectionResult{}

	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(strings.ToUpper(line), "COMPLETED:") {
			value := strings.TrimSpace(strings.ToLower(strings.TrimPrefix(strings.ToUpper(line), "COMPLETED:")))
			result.TaskCompleted = value == "yes" || value == "true"
		}

		if strings.HasPrefix(strings.ToUpper(line), "REASON:") {
			result.Reason = strings.TrimSpace(strings.TrimPrefix(strings.ToUpper(line), "REASON:"))
		}

		if strings.HasPrefix(strings.ToUpper(line), "RETRY:") {
			value := strings.TrimSpace(strings.ToLower(strings.TrimPrefix(strings.ToUpper(line), "RETRY:")))
			result.ShouldRetry = value == "yes" || value == "true"
		}
	}

	return result
}

func (sre *ReflectionEngine) simpleEvaluation(plan *types.ExecutionPlan) *ReflectionResult {
	plan.RLock()
	defer plan.RUnlock()

	successCount := 0
	hasAttemptCompletion := false
	completionSucceeded := false

	for _, step := range plan.Steps {
		if step.Status == types.StepStatusCompleted {
			successCount++
		}

		if step.ToolName == "attempt_completion" {
			hasAttemptCompletion = true
			if step.Status == types.StepStatusCompleted {
				completionSucceeded = true
			}
		}
	}

	totalSteps := len(plan.Steps)
	successRate := float64(successCount) / float64(totalSteps)

	taskCompleted := completionSucceeded || (successRate >= 0.8 && !hasAttemptCompletion)

	var reason string
	var shouldRetry bool

	if taskCompleted {
		reason = fmt.Sprintf("Task completed successfully (%d/%d steps)", successCount, totalSteps)
		shouldRetry = false
	} else if successRate >= 0.5 {
		reason = fmt.Sprintf("Partial completion (%d/%d steps) - may need continuation", successCount, totalSteps)
		shouldRetry = true
	} else {
		reason = fmt.Sprintf("Low completion rate (%d/%d steps) - significant issues", successCount, totalSteps)
		shouldRetry = false
	}

	return &ReflectionResult{
		TaskCompleted: taskCompleted,
		Reason:        reason,
		ShouldRetry:   shouldRetry,
	}
}
