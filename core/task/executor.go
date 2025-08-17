package task

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/vadiminshakov/autonomy/core/tools"
	"github.com/vadiminshakov/autonomy/core/types"
	"github.com/vadiminshakov/autonomy/ui"
)

type ExecutionResult struct {
	StepID   string
	ToolName string
	Result   string
	Error    error
	Duration time.Duration
}

type ParallelExecutor struct {
	maxWorkers int
	timeout    time.Duration
	planner    *Planner
}

func NewParallelExecutor(maxWorkers int, timeout time.Duration) *ParallelExecutor {
	return &ParallelExecutor{
		maxWorkers: maxWorkers,
		timeout:    timeout,
		planner:    NewPlanner(),
	}
}

func (pe *ParallelExecutor) ExecutePlan(ctx context.Context, plan *types.ExecutionPlan) error {
	for !pe.planner.IsCompleted(plan) {
		readySteps := pe.planner.GetReadySteps(plan)
		if len(readySteps) == 0 {
			if pe.planner.HasFailures(plan) {
				return fmt.Errorf("execution plan failed - some steps could not be completed")
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}

		if err := pe.executeStepsParallel(ctx, plan, readySteps); err != nil {
			return fmt.Errorf("parallel execution failed: %v", err)
		}
	}

	if pe.planner.HasFailures(plan) {
		return fmt.Errorf("execution plan completed with failures")
	}

	return nil
}

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

	var toolErrors []error
	completedCount := 0

	for result := range resultChan {
		completedCount++

		if result.Error != nil {
			toolErrors = append(toolErrors, fmt.Errorf("step %s failed: %v", result.StepID, result.Error))
			pe.planner.SetStepResult(plan, result.StepID, result.Result, result.Error)

			// provide helpful error context for common issues
			if strings.Contains(result.Error.Error(), "no such file") {
				fmt.Printf("%s Hint: File not found. Use 'find_files' or 'get_project_structure' to verify file location\n", ui.Warning(""))
			} else if strings.Contains(result.Error.Error(), "parameter") && strings.Contains(result.Error.Error(), "required") {
				fmt.Printf("%s Hint: Missing required parameter. Check tool description for required arguments\n", ui.Warning(""))
			}
		} else {
			pe.planner.SetStepResult(plan, result.StepID, result.Result, nil)

			// check for empty results that might indicate a problem
			if result.Result == "" && !isSilentTool(result.ToolName) {
				fmt.Printf("%s Tool %s returned empty result - verify if this was expected\n", ui.Warning(""), result.ToolName)
			}

			if isSilentTool(result.ToolName) {
				step := pe.findStepInPlan(plan, result.StepID)
				if step != nil {
					summary := silentToolSummary(result.ToolName, step.Args, result.Result)
					fmt.Println(ui.Success(result.ToolName) + summary)
				}
			} else {
				fmt.Println(ui.Success(fmt.Sprintf("%s completed", result.ToolName)))
				if result.Result != "" {
					truncatedResult := limitToolOutput(result.Result)
					fmt.Printf("%s\n", ui.Dim("Result: "+truncatedResult))
				}
			}
		}
	}

	if len(toolErrors) > 0 {
		return fmt.Errorf("parallel execution failed with %d errors: %s", len(toolErrors), errors.Join(toolErrors...))
	}

	return nil
}

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

func (pe *ParallelExecutor) executeStep(ctx context.Context, step *types.ExecutionStep) ExecutionResult {
	startTime := time.Now()

	timeout := pe.getToolTimeout(step.ToolName)
	toolCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

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

	return 30 * time.Second
}

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
