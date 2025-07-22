package task

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/vadiminshakov/autonomy/core/entity"
	"github.com/vadiminshakov/autonomy/core/types"
)


// Planner creates and manages execution plans
type Planner struct {
	mu sync.RWMutex
}

// NewPlanner creates a new execution planner
func NewPlanner() *Planner {
	return &Planner{}
}

// CreatePlan creates an execution plan from tool calls
func (p *Planner) CreatePlan(toolCalls []entity.ToolCall) *types.ExecutionPlan {
	p.mu.Lock()
	defer p.mu.Unlock()

	plan := &types.ExecutionPlan{
		Steps: make([]*types.ExecutionStep, 0, len(toolCalls)),
	}

	// convert tool calls to execution steps
	for i, call := range toolCalls {
		step := &types.ExecutionStep{
			ID:           fmt.Sprintf("step_%d", i+1),
			ToolName:     call.Name,
			Args:         call.Args,
			Dependencies: p.inferDependencies(call, toolCalls[:i]),
			Status:       types.StepStatusPending,
			Category:     p.categorizeStep(call.Name),
		}

		// populate files affected based on tool arguments
		if files := p.extractFilesAffected(call); len(files) > 0 {
			step.FilesAffected = files
		}

		plan.Steps = append(plan.Steps, step)
	}

	// analyze dependencies and create parallel groups
	plan.ParallelGroups = p.AnalyzeDependencies(plan)

	return plan
}

// inferDependencies infers dependencies based on tool types and arguments
//
//nolint:gocyclo
func (p *Planner) inferDependencies(current entity.ToolCall, previous []entity.ToolCall) []string {
	var deps []string

	// Rules for dependency inference:
	switch current.Name {
	case "analyze_code_go":
		// analyze_code_go depends on read_file for the same file
		if path, ok := current.Args["path"].(string); ok {
			for i, prev := range previous {
				if prev.Name == "read_file" {
					if prevPath, ok := prev.Args["path"].(string); ok && prevPath == path {
						deps = append(deps, fmt.Sprintf("step_%d", i+1))
					}
				}
			}
		}

	case "apply_diff":
		// apply_diff depends on read_file for the same file
		if path, ok := current.Args["path"].(string); ok {
			for i, prev := range previous {
				if prev.Name == "read_file" {
					if prevPath, ok := prev.Args["path"].(string); ok && prevPath == path {
						deps = append(deps, fmt.Sprintf("step_%d", i+1))
					}
				}
			}
		}

	case "go_test", "go_vet":
		// testing tools depend on any file modifications
		for i, prev := range previous {
			if prev.Name == "write_file" || prev.Name == "apply_diff" {
				deps = append(deps, fmt.Sprintf("step_%d", i+1))
			}
		}

	case "attempt_completion":
		// attempt_completion depends on all analysis and modification steps
		for i, prev := range previous {
			if isAnalysisOrModificationTool(prev.Name) {
				deps = append(deps, fmt.Sprintf("step_%d", i+1))
			}
		}
	}

	return deps
}

// isAnalysisOrModificationTool checks if a tool performs analysis or modification
func isAnalysisOrModificationTool(toolName string) bool {
	analysisTools := map[string]bool{
		"read_file":             true,
		"analyze_code_go":       true,
		"search_dir":            true,
		"search_index":          true,
		"get_project_structure": true,
		"write_file":            true,
		"apply_diff":            true,
		"go_test":               true,
		"go_vet":                true,
	}
	return analysisTools[toolName]
}

// AnalyzeDependencies analyzes the execution plan and identifies parallel execution groups
func (p *Planner) AnalyzeDependencies(plan *types.ExecutionPlan) [][]string {
	var parallelGroups [][]string
	processed := make(map[string]bool)

	for _, step := range plan.Steps {
		if processed[step.ID] {
			continue
		}

		// find all steps that can run in parallel with this step
		parallelGroup := []string{step.ID}
		processed[step.ID] = true

		for _, otherStep := range plan.Steps {
			if processed[otherStep.ID] {
				continue
			}

			// check if steps can run in parallel
			if p.canRunInParallel(step, otherStep, plan.Steps) {
				parallelGroup = append(parallelGroup, otherStep.ID)
				processed[otherStep.ID] = true
			}
		}

		parallelGroups = append(parallelGroups, parallelGroup)
	}

	return parallelGroups
}

// canRunInParallel checks if two steps can be executed in parallel
func (p *Planner) canRunInParallel(step1, step2 *types.ExecutionStep, allSteps []*types.ExecutionStep) bool {
	// steps cannot run in parallel if one depends on the other
	if p.hasDependency(step1, step2, allSteps) || p.hasDependency(step2, step1, allSteps) {
		return false
	}

	// steps that modify the same resource cannot run in parallel
	if p.conflictingResources(step1, step2) {
		return false
	}

	// some tools are inherently sequential
	if p.isSequentialTool(step1.ToolName) || p.isSequentialTool(step2.ToolName) {
		return false
	}

	return true
}

// hasDependency checks if step1 depends on step2 (directly or indirectly)
func (p *Planner) hasDependency(step1, step2 *types.ExecutionStep, allSteps []*types.ExecutionStep) bool {
	// direct dependency
	for _, dep := range step1.Dependencies {
		if dep == step2.ID {
			return true
		}
	}

	// indirect dependency (recursive check)
	for _, dep := range step1.Dependencies {
		for _, step := range allSteps {
			if step.ID == dep {
				if p.hasDependency(step, step2, allSteps) {
					return true
				}
			}
		}
	}

	return false
}

// conflictingResources checks if two steps access conflicting resources
func (p *Planner) conflictingResources(step1, step2 *types.ExecutionStep) bool {
	// check for file path conflicts
	path1 := p.getFilePath(step1)
	path2 := p.getFilePath(step2)

	if path1 != "" && path2 != "" && path1 == path2 {
		// same file - check if both are write operations
		if p.isWriteOperation(step1.ToolName) && p.isWriteOperation(step2.ToolName) {
			return true
		}
	}

	return false
}

// getFilePath extracts file path from step arguments
func (p *Planner) getFilePath(step *types.ExecutionStep) string {
	if path, ok := step.Args["path"].(string); ok {
		return path
	}
	if file, ok := step.Args["file"].(string); ok {
		return file
	}
	return ""
}

// isWriteOperation checks if a tool performs write operations
func (p *Planner) isWriteOperation(toolName string) bool {
	writeTools := map[string]bool{
		"write_file": true,
		"apply_diff": true,
	}
	return writeTools[toolName]
}

// isSequentialTool checks if a tool must be executed sequentially
func (p *Planner) isSequentialTool(toolName string) bool {
	sequentialTools := map[string]bool{
		"attempt_completion": true,
		"execute_command":    true, // commands might have side effects
	}
	return sequentialTools[toolName]
}

// GetParallelGroups returns the parallel execution groups
func (p *Planner) GetParallelGroups(plan *types.ExecutionPlan) [][]string {
	plan.Mu.RLock()
	defer plan.Mu.RUnlock()
	return plan.ParallelGroups
}

// GetReadySteps returns steps that are ready to execute (all dependencies completed)
func (p *Planner) GetReadySteps(plan *types.ExecutionPlan) []*types.ExecutionStep {
	plan.Mu.RLock()
	defer plan.Mu.RUnlock()

	var readySteps []*types.ExecutionStep

	for _, step := range plan.Steps {
		if step.Status != types.StepStatusPending {
			continue
		}

		// check if all dependencies are completed
		allDepsCompleted := true
		for _, depID := range step.Dependencies {
			depStep := p.findStepByID(plan, depID)
			if depStep == nil || depStep.Status != types.StepStatusCompleted {
				allDepsCompleted = false
				break
			}
		}

		if allDepsCompleted {
			readySteps = append(readySteps, step)
		}
	}

	return readySteps
}

// findStepByID finds a step by its ID
func (p *Planner) findStepByID(plan *types.ExecutionPlan, id string) *types.ExecutionStep {
	for _, step := range plan.Steps {
		if step.ID == id {
			return step
		}
	}
	return nil
}

// UpdateStepStatus updates the status of a step
func (p *Planner) UpdateStepStatus(plan *types.ExecutionPlan, stepID string, status types.StepStatus) {
	plan.Mu.Lock()
	defer plan.Mu.Unlock()

	for _, step := range plan.Steps {
		if step.ID == stepID {
			step.Status = status
			switch status {
			case types.StepStatusRunning:
				now := time.Now()
				step.StartTime = &now
			case types.StepStatusCompleted, types.StepStatusFailed:
				now := time.Now()
				step.EndTime = &now
			}

			break
		}
	}
}

// SetStepResult sets the result and error for a step
func (p *Planner) SetStepResult(plan *types.ExecutionPlan, stepID string, result string, err error) {
	plan.Mu.Lock()
	defer plan.Mu.Unlock()

	for _, step := range plan.Steps {
		if step.ID == stepID {
			step.Result = result
			step.Error = err
			if err != nil {
				step.Status = types.StepStatusFailed
			} else {
				step.Status = types.StepStatusCompleted
			}
			now := time.Now()
			step.EndTime = &now
			break
		}
	}
}

// GetPlanSummary returns a human-readable summary of the execution plan
func (p *Planner) GetPlanSummary(plan *types.ExecutionPlan) string {
	plan.Mu.RLock()
	defer plan.Mu.RUnlock()

	var summary strings.Builder
	summary.WriteString("Execution Plan:\n")

	for i, group := range plan.ParallelGroups {
		if len(group) == 1 {
			step := p.findStepByID(plan, group[0])
			if step != nil {
				summary.WriteString(fmt.Sprintf("  %d. %s (%s)\n", i+1, step.ToolName, step.Status))
			}
		} else {
			summary.WriteString(fmt.Sprintf("  %d. Parallel group:\n", i+1))
			for _, stepID := range group {
				step := p.findStepByID(plan, stepID)
				if step != nil {
					summary.WriteString(fmt.Sprintf("     - %s (%s)\n", step.ToolName, step.Status))
				}
			}
		}
	}

	return summary.String()
}

// IsCompleted checks if the entire plan is completed
func (p *Planner) IsCompleted(plan *types.ExecutionPlan) bool {
	plan.Mu.RLock()
	defer plan.Mu.RUnlock()

	for _, step := range plan.Steps {
		if step.Status != types.StepStatusCompleted && step.Status != types.StepStatusFailed {
			return false
		}
	}
	return true
}

// HasFailures checks if any step in the plan has failed
func (p *Planner) HasFailures(plan *types.ExecutionPlan) bool {
	plan.Mu.RLock()
	defer plan.Mu.RUnlock()

	for _, step := range plan.Steps {
		if step.Status == types.StepStatusFailed {
			return true
		}
	}
	return false
}

// categorizeStep categorizes a step based on its tool name
func (p *Planner) categorizeStep(toolName string) string {
	switch toolName {
	case "read_file", "search_dir", "search_index", "get_project_structure", "analyze_code_go", "get_function", "get_type", "get_package_info":
		return "analysis"
	case "write_file", "apply_diff", "rename_symbol_go", "extract_function_go", "inline_function_go":
		return "modification"
	case "go_test", "go_vet":
		return "test"
	case "execute_command":
		return "execution"
	case "attempt_completion":
		return "completion"
	default:
		return "other"
	}
}

// extractFilesAffected extracts files that will be affected by a tool call
func (p *Planner) extractFilesAffected(call entity.ToolCall) []string {
	var files []string

	// extract file paths from different argument names
	if path, ok := call.Args["path"].(string); ok && path != "" {
		files = append(files, path)
	}
	if file, ok := call.Args["file"].(string); ok && file != "" {
		files = append(files, file)
	}
	if fileName, ok := call.Args["fileName"].(string); ok && fileName != "" {
		files = append(files, fileName)
	}
	if filePath, ok := call.Args["file_path"].(string); ok && filePath != "" {
		files = append(files, filePath)
	}

	return files
}
