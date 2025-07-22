package reflection

import (
	"context"
	"errors"
	"testing"

	"github.com/vadiminshakov/autonomy/core/entity"
	"github.com/vadiminshakov/autonomy/core/types"
)

// MockAIClient для тестирования
type MockAIClient struct {
	response *entity.AIResponse
	err      error
}

func (m *MockAIClient) GenerateCode(ctx context.Context, promptData entity.PromptData) (*entity.AIResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func TestSimpleReflectionEngine_EvaluateCompletion_Success(t *testing.T) {
	mockClient := &MockAIClient{
		response: &entity.AIResponse{
			Content: `COMPLETED: yes
REASON: Task completed successfully
RETRY: no`,
		},
	}

	engine := NewReflectionEngine(mockClient)

	// Create a successful execution plan
	plan := &types.ExecutionPlan{
		Steps: []*types.ExecutionStep{
			{
				ID:       "step_1",
				ToolName: "read_file",
				Status:   types.StepStatusCompleted,
			},
			{
				ID:       "step_2",
				ToolName: "write_file",
				Status:   types.StepStatusCompleted,
			},
			{
				ID:       "step_3",
				ToolName: "attempt_completion",
				Status:   types.StepStatusCompleted,
			},
		},
	}

	result, err := engine.EvaluateCompletion(context.Background(), plan, "Test task")

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if !result.TaskCompleted {
		t.Error("Expected task to be completed")
	}

	if result.ShouldRetry {
		t.Error("Expected no retry needed")
	}

	if result.Reason == "" {
		t.Error("Expected reason to be provided")
	}
}

func TestSimpleReflectionEngine_EvaluateCompletion_Failure(t *testing.T) {
	mockClient := &MockAIClient{
		response: &entity.AIResponse{
			Content: `COMPLETED: no
REASON: Multiple errors occurred
RETRY: yes`,
		},
	}

	engine := NewReflectionEngine(mockClient)

	// Create a failed execution plan
	plan := &types.ExecutionPlan{
		Steps: []*types.ExecutionStep{
			{
				ID:       "step_1",
				ToolName: "read_file",
				Status:   types.StepStatusCompleted,
			},
			{
				ID:       "step_2",
				ToolName: "write_file",
				Status:   types.StepStatusFailed,
				Error:    errors.New("write failed"),
			},
		},
	}

	result, err := engine.EvaluateCompletion(context.Background(), plan, "Test task")

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.TaskCompleted {
		t.Error("Expected task to not be completed")
	}

	if !result.ShouldRetry {
		t.Error("Expected retry to be suggested")
	}
}

func TestSimpleReflectionEngine_EvaluateCompletion_AIError_Fallback(t *testing.T) {
	mockClient := &MockAIClient{
		err: errors.New("AI service unavailable"),
	}

	engine := NewReflectionEngine(mockClient)

	// Create a mostly successful execution plan
	plan := &types.ExecutionPlan{
		Steps: []*types.ExecutionStep{
			{
				ID:       "step_1",
				ToolName: "read_file",
				Status:   types.StepStatusCompleted,
			},
			{
				ID:       "step_2",
				ToolName: "write_file",
				Status:   types.StepStatusCompleted,
			},
			{
				ID:       "step_3",
				ToolName: "attempt_completion",
				Status:   types.StepStatusCompleted,
			},
		},
	}

	result, err := engine.EvaluateCompletion(context.Background(), plan, "Test task")

	// Should not return error even if AI fails - should use fallback
	if err != nil {
		t.Fatalf("Expected no error from fallback, got %v", err)
	}

	// With attempt_completion succeeded, should be considered completed
	if !result.TaskCompleted {
		t.Error("Expected task to be completed via fallback evaluation")
	}
}

func TestSimpleReflectionEngine_SimpleEvaluation_RuleBasedLogic(t *testing.T) {
	engine := NewReflectionEngine(nil)

	tests := []struct {
		name          string
		plan          *types.ExecutionPlan
		expectedDone  bool
		expectedRetry bool
	}{
		{
			name: "Perfect success with attempt_completion",
			plan: &types.ExecutionPlan{
				Steps: []*types.ExecutionStep{
					{ToolName: "read_file", Status: types.StepStatusCompleted},
					{ToolName: "write_file", Status: types.StepStatusCompleted},
					{ToolName: "attempt_completion", Status: types.StepStatusCompleted},
				},
			},
			expectedDone:  true,
			expectedRetry: false,
		},
		{
			name: "High success rate without attempt_completion",
			plan: &types.ExecutionPlan{
				Steps: []*types.ExecutionStep{
					{ToolName: "read_file", Status: types.StepStatusCompleted},
					{ToolName: "write_file", Status: types.StepStatusCompleted},
					{ToolName: "analyze_code", Status: types.StepStatusCompleted},
					{ToolName: "search", Status: types.StepStatusCompleted},
				},
			},
			expectedDone:  true,
			expectedRetry: false,
		},
		{
			name: "Moderate success - should retry",
			plan: &types.ExecutionPlan{
				Steps: []*types.ExecutionStep{
					{ToolName: "read_file", Status: types.StepStatusCompleted},
					{ToolName: "write_file", Status: types.StepStatusFailed},
					{ToolName: "analyze_code", Status: types.StepStatusCompleted},
				},
			},
			expectedDone:  false,
			expectedRetry: true,
		},
		{
			name: "Low success rate - no retry",
			plan: &types.ExecutionPlan{
				Steps: []*types.ExecutionStep{
					{ToolName: "read_file", Status: types.StepStatusFailed},
					{ToolName: "write_file", Status: types.StepStatusFailed},
					{ToolName: "analyze_code", Status: types.StepStatusCompleted},
				},
			},
			expectedDone:  false,
			expectedRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.simpleEvaluation(tt.plan)

			if result.TaskCompleted != tt.expectedDone {
				t.Errorf("Expected TaskCompleted=%v, got %v", tt.expectedDone, result.TaskCompleted)
			}

			if result.ShouldRetry != tt.expectedRetry {
				t.Errorf("Expected ShouldRetry=%v, got %v", tt.expectedRetry, result.ShouldRetry)
			}

			if result.Reason == "" {
				t.Error("Expected reason to be provided")
			}
		})
	}
}

func TestSimpleReflectionEngine_ParseResponse(t *testing.T) {
	engine := NewReflectionEngine(nil)

	tests := []struct {
		name             string
		response         string
		expectedComplete bool
		expectedRetry    bool
		expectedReason   string
	}{
		{
			name: "Standard format",
			response: `COMPLETED: yes
REASON: All steps completed successfully
RETRY: no`,
			expectedComplete: true,
			expectedRetry:    false,
			expectedReason:   "ALL STEPS COMPLETED SUCCESSFULLY",
		},
		{
			name: "Case insensitive",
			response: `completed: YES
reason: Task done
retry: FALSE`,
			expectedComplete: true,
			expectedRetry:    false,
			expectedReason:   "TASK DONE",
		},
		{
			name: "With extra text",
			response: `Some AI explanation here...
COMPLETED: no
REASON: Had some issues
RETRY: true
More text after...`,
			expectedComplete: false,
			expectedRetry:    true,
			expectedReason:   "HAD SOME ISSUES",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.parseResponse(tt.response)

			if result.TaskCompleted != tt.expectedComplete {
				t.Errorf("Expected TaskCompleted=%v, got %v", tt.expectedComplete, result.TaskCompleted)
			}

			if result.ShouldRetry != tt.expectedRetry {
				t.Errorf("Expected ShouldRetry=%v, got %v", tt.expectedRetry, result.ShouldRetry)
			}

			if result.Reason != tt.expectedReason {
				t.Errorf("Expected Reason=%q, got %q", tt.expectedReason, result.Reason)
			}
		})
	}
}
