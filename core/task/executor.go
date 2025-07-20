package task

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vadiminshakov/autonomy/core/entity"
	"github.com/vadiminshakov/autonomy/core/tools"
	"github.com/vadiminshakov/autonomy/ui"
)

// ExecutionResult represents the result of a tool execution
type ExecutionResult struct {
	StepID   string
	ToolName string
	Result   string
	Error    error
	Duration time.Duration
}

// ParallelExecutor manages parallel execution of tools
type ParallelExecutor struct {
	maxWorkers int
	timeout    time.Duration
	planner    *Planner
}

// NewParallelExecutor creates a new parallel executor
func NewParallelExecutor(maxWorkers int, timeout time.Duration) *ParallelExecutor {
	return &ParallelExecutor{
		maxWorkers: maxWorkers,
		timeout:    timeout,
		planner:    NewPlanner(),
	}
}

// ExecutePlan executes an execution plan with parallel processing
func (pe *ParallelExecutor) ExecutePlan(ctx context.Context, plan *ExecutionPlan) error {
	fmt.Println(ui.Tool(fmt.Sprintf("Executing plan with %d steps...", len(plan.Steps))))

	for !pe.planner.IsCompleted(plan) {
		// get steps that are ready to execute
		readySteps := pe.planner.GetReadySteps(plan)
		if len(readySteps) == 0 {
			// check if we're stuck (no ready steps but plan not completed)
			if pe.planner.HasFailures(plan) {
				return fmt.Errorf("execution plan failed - some steps could not be completed")
			}
			// wait a bit and check again (shouldn't happen with proper dependency analysis)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// execute ready steps in parallel
		if err := pe.executeStepsParallel(ctx, plan, readySteps); err != nil {
			return fmt.Errorf("parallel execution failed: %v", err)
		}
	}

	if pe.planner.HasFailures(plan) {
		return fmt.Errorf("execution plan completed with failures")
	}

	// generate and display task summary using existing task state system
	summaryResult, summaryErr := tools.Execute("get_task_summary", map[string]any{})

	fmt.Println(ui.Success("Plan execution completed successfully"))
	if summaryErr == nil && summaryResult != "" {
		fmt.Println(ui.Info(summaryResult))
	}
	return nil
}

// executeStepsParallel executes multiple steps in parallel
func (pe *ParallelExecutor) executeStepsParallel(ctx context.Context, plan *ExecutionPlan, steps []*ExecutionStep) error {
	if len(steps) == 0 {
		return nil
	}

	// Create worker pool
	workerCount := pe.maxWorkers
	if len(steps) < workerCount {
		workerCount = len(steps)
	}

	// Channels for work distribution and result collection
	stepChan := make(chan *ExecutionStep, len(steps))
	resultChan := make(chan ExecutionResult, len(steps))

	// Context with timeout for the entire parallel execution
	execCtx, cancel := context.WithTimeout(ctx, pe.timeout)
	defer cancel()

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go pe.worker(execCtx, &wg, stepChan, resultChan)
	}

	// Send steps to workers
	for _, step := range steps {
		pe.planner.UpdateStepStatus(plan, step.ID, StepStatusRunning)
		stepChan <- step
	}
	close(stepChan)

	// Collect results
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Process results as they come in
	var errors []error
	completedCount := 0

	for result := range resultChan {
		completedCount++

		if result.Error != nil {
			errors = append(errors, fmt.Errorf("step %s failed: %v", result.StepID, result.Error))
			pe.planner.SetStepResult(plan, result.StepID, result.Result, result.Error)
			fmt.Println(ui.Error(fmt.Sprintf("❌ %s failed: %v", result.ToolName, result.Error)))
		} else {
			pe.planner.SetStepResult(plan, result.StepID, result.Result, nil)

			// Show result based on tool type
			if isSilentTool(result.ToolName) {
				// For silent tools, show summary
				step := pe.findStepInPlan(plan, result.StepID)
				if step != nil {
					summary := silentToolSummary(result.ToolName, step.Args, result.Result)
					fmt.Println(ui.Success("✅ "+result.ToolName) + summary)
				}
			} else {
				fmt.Println(ui.Success(fmt.Sprintf("✅ %s completed", result.ToolName)))
				if result.Result != "" {
					truncatedResult := limitToolOutput(result.Result)
					fmt.Printf("%s\n", ui.Dim("Result: "+truncatedResult))
				}
			}
		}

		fmt.Printf("%s\n", ui.Info(fmt.Sprintf("Progress: %d/%d steps completed", completedCount, len(steps))))
	}

	// Return first error if any occurred
	if len(errors) > 0 {
		return errors[0]
	}

	return nil
}

// worker is a worker goroutine that executes steps
func (pe *ParallelExecutor) worker(
	ctx context.Context,
	wg *sync.WaitGroup,
	stepChan <-chan *ExecutionStep,
	resultChan chan<- ExecutionResult,
) {
	defer wg.Done()

	for step := range stepChan {
		select {
		case <-ctx.Done():
			resultChan <- ExecutionResult{
				StepID:   step.ID,
				ToolName: step.ToolName,
				Error:    ctx.Err(),
			}
			return
		default:
			// Execute the step
			result := pe.executeStep(ctx, step)
			resultChan <- result
		}
	}
}

// executeStep executes a single step
func (pe *ParallelExecutor) executeStep(ctx context.Context, step *ExecutionStep) ExecutionResult {
	startTime := time.Now()

	// Get appropriate timeout for this tool
	timeout := pe.getToolTimeout(step.ToolName)
	toolCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute the tool
	result, err := pe.executeToolWithTimeout(toolCtx, step.ToolName, step.Args)
	duration := time.Since(startTime)

	return ExecutionResult{
		StepID:   step.ID,
		ToolName: step.ToolName,
		Result:   result,
		Error:    err,
		Duration: duration,
	}
}

// executeToolWithTimeout executes a tool with timeout handling
func (pe *ParallelExecutor) executeToolWithTimeout(ctx context.Context, toolName string, args map[string]any) (string, error) {
	resultChan := make(chan struct {
		res string
		err error
	}, 1)

	go func() {
		res, err := tools.Execute(toolName, args)
		resultChan <- struct {
			res string
			err error
		}{res, err}
	}()

	select {
	case result := <-resultChan:
		return result.res, result.err
	case <-ctx.Done():
		return "", fmt.Errorf("tool %s timed out", toolName)
	}
}

// getToolTimeout returns appropriate timeout for different tool types
func (pe *ParallelExecutor) getToolTimeout(toolName string) time.Duration {
	// Long-running tools that need more time
	longRunningTools := map[string]time.Duration{
		"build_index":     5 * time.Minute,
		"execute_command": 3 * time.Minute,
		"go_test":         2 * time.Minute,
		"search_index":    2 * time.Minute,
		"analyze_code_go": 1 * time.Minute,
	}

	// Check if this tool needs a longer timeout
	if timeout, ok := longRunningTools[toolName]; ok {
		return timeout
	}

	// Default timeout for regular tools
	return 30 * time.Second
}

// findStepInPlan finds a step by id in the execution plan
func (pe *ParallelExecutor) findStepInPlan(plan *ExecutionPlan, stepID string) *ExecutionStep {
	plan.mu.RLock()
	defer plan.mu.RUnlock()

	for _, step := range plan.Steps {
		if step.ID == stepID {
			return step
		}
	}
	return nil
}

// ExecuteSequential executes steps sequentially (fallback for when parallel execution is not suitable)
func (pe *ParallelExecutor) ExecuteSequential(ctx context.Context, toolCalls []entity.ToolCall) (bool, error) {
	for i, call := range toolCalls {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}

		fmt.Printf("%s\n", ui.Blue(fmt.Sprintf("📋 Tool %d/%d: %s", i+1, len(toolCalls), call.Name)))

		// Start spinner for tool execution
		spinner := ui.ShowToolExecution(call.Name)

		result, err := pe.executeToolWithTimeout(ctx, call.Name, call.Args)

		spinner.Stop()

		// Handle result
		if err != nil {
			fmt.Println(ui.Error(fmt.Sprintf("Error running %s: %v", call.Name, err)))
			if result != "" && !isSilentTool(call.Name) {
				fmt.Printf("%s\n", ui.Dim("Result: "+result))
			}
			// Continue with next tool even if this one failed
			continue
		}

		// Success path
		if isSilentTool(call.Name) {
			summary := silentToolSummary(call.Name, call.Args, result)
			fmt.Println(ui.Success("Done "+call.Name) + summary)
		} else {
			fmt.Println(ui.Success("Done " + call.Name))

			if result != "" {
				if call.Name == "attempt_completion" {
					// Try to generate and include task summary using existing task state
					summaryResult, summaryErr := pe.executeToolWithTimeout(ctx, "get_task_summary", map[string]any{})
					if summaryErr == nil && summaryResult != "" {
						enhancedResult := result + "\n\n" + summaryResult
						fmt.Println(ui.Info(enhancedResult))
					} else {
						fmt.Println(ui.Info(result))
					}
					return true, nil // Task completed
				} else {
					result = limitToolOutput(result)
					fmt.Printf("%s\n", ui.Dim("Result: "+result))
				}
			}
		}

		if call.Name == "attempt_completion" {
			return true, nil
		}
	}

	return false, nil
}

// shouldUsePlanning determines if a task should use planning based on complexity
func (pe *ParallelExecutor) ShouldUsePlanning(toolCalls []entity.ToolCall) bool {
	// Use planning for tasks with multiple tools
	if len(toolCalls) >= 3 {
		return true
	}

	// Use planning for tasks that involve file analysis and modification
	hasAnalysis := false
	hasModification := false

	for _, call := range toolCalls {
		switch call.Name {
		case "read_file", "analyze_code_go", "search_dir", "search_index":
			hasAnalysis = true
		case "write_file", "apply_diff":
			hasModification = true
		}
	}

	return hasAnalysis && hasModification
}

// GetExecutionStats returns statistics about the execution
func (pe *ParallelExecutor) GetExecutionStats(plan *ExecutionPlan) map[string]any {
	plan.mu.RLock()
	defer plan.mu.RUnlock()

	stats := map[string]any{
		"total_steps":     len(plan.Steps),
		"completed_steps": 0,
		"failed_steps":    0,
		"parallel_groups": len(plan.ParallelGroups),
	}

	var totalDuration time.Duration

	for _, step := range plan.Steps {
		switch step.Status {
		case StepStatusCompleted:
			stats["completed_steps"] = stats["completed_steps"].(int) + 1
			if step.StartTime != nil && step.EndTime != nil {
				totalDuration += step.EndTime.Sub(*step.StartTime)
			}
		case StepStatusFailed:
			stats["failed_steps"] = stats["failed_steps"].(int) + 1
		}
	}

	stats["total_duration_ms"] = totalDuration.Milliseconds()
	return stats
}
