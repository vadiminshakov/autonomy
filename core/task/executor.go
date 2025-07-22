package task

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/vadiminshakov/autonomy/core/tools"
	"github.com/vadiminshakov/autonomy/core/types"
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
func (pe *ParallelExecutor) ExecutePlan(ctx context.Context, plan *types.ExecutionPlan) error {
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
func (pe *ParallelExecutor) executeStepsParallel(ctx context.Context, plan *types.ExecutionPlan, steps []*types.ExecutionStep) error {
	if len(steps) == 0 {
		return nil
	}

	workerCount := pe.maxWorkers
	if len(steps) < workerCount {
		workerCount = len(steps)
	}

	stepChan := make(chan *types.ExecutionStep, len(steps))
	resultChan := make(chan ExecutionResult, len(steps))

	execCtx, cancel := context.WithTimeout(ctx, pe.timeout)
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go pe.worker(execCtx, &wg, stepChan, resultChan)
	}

	for _, step := range steps {
		pe.planner.UpdateStepStatus(plan, step.ID, types.StepStatusRunning)
		stepChan <- step
	}
	close(stepChan)

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Process results as they come in
	var toolErrors []error
	completedCount := 0

	for result := range resultChan {
		completedCount++

		if result.Error != nil {
			toolErrors = append(toolErrors, fmt.Errorf("step %s failed: %v", result.StepID, result.Error))
			pe.planner.SetStepResult(plan, result.StepID, result.Result, result.Error)
		} else {
			pe.planner.SetStepResult(plan, result.StepID, result.Result, nil)

			// show result based on tool type
			if isSilentTool(result.ToolName) {
				// for silent tools, show summary
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

	if len(toolErrors) > 0 {
		return fmt.Errorf("parallel execution failed with %d errors: %s", len(toolErrors), errors.Join(toolErrors...))
	}

	return nil
}

// worker is a worker goroutine that executes steps
func (pe *ParallelExecutor) worker(
	ctx context.Context,
	wg *sync.WaitGroup,
	stepChan <-chan *types.ExecutionStep,
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
			result := pe.executeStep(ctx, step)
			resultChan <- result
		}
	}
}

// executeStep executes a single step
func (pe *ParallelExecutor) executeStep(ctx context.Context, step *types.ExecutionStep) ExecutionResult {
	startTime := time.Now()

	// get appropriate timeout for this tool
	timeout := pe.getToolTimeout(step.ToolName)
	toolCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// execute the tool
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
	longRunningTools := map[string]time.Duration{
		"build_index":     5 * time.Minute,
		"execute_command": 3 * time.Minute,
		"go_test":         2 * time.Minute,
		"search_index":    2 * time.Minute,
		"analyze_code_go": 1 * time.Minute,
	}

	if timeout, ok := longRunningTools[toolName]; ok {
		return timeout
	}

	// default timeout for regular tools
	return 30 * time.Second
}

// findStepInPlan finds a step by id in the execution plan
func (pe *ParallelExecutor) findStepInPlan(plan *types.ExecutionPlan, stepID string) *types.ExecutionStep {
	plan.Mu.RLock()
	defer plan.Mu.RUnlock()

	for _, step := range plan.Steps {
		if step.ID == stepID {
			return step
		}
	}
	return nil
}
