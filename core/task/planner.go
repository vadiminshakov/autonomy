package task

import (
	"encoding/json"
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

	for i, call := range toolCalls {
		var args map[string]any
		if call.Function.Arguments != "" {
			json.Unmarshal([]byte(call.Function.Arguments), &args)
		} else if call.Args != nil {
			args = call.Args
		}

		toolName := call.Function.Name
		if toolName == "" {
			toolName = call.Name
		}

		step := &types.ExecutionStep{
			ID:           fmt.Sprintf("step_%d", i+1),
			ToolName:     toolName,
			Args:         args,
			Dependencies: p.inferDependencies(call, toolCalls[:i]),
			Status:       types.StepStatusPending,
			Category:     p.categorizeStep(toolName),
		}

		if files := p.extractFilesAffected(call); len(files) > 0 {
			step.FilesAffected = files
		}

		plan.Steps = append(plan.Steps, step)
	}

	plan.ParallelGroups = p.AnalyzeDependencies(plan)

	return plan
}

func (p *Planner) inferDependencies(current entity.ToolCall, previous []entity.ToolCall) []string {
	var deps []string

	// получаем аргументы текущего вызова
	var currentArgs map[string]any
	if current.Function.Arguments != "" {
		json.Unmarshal([]byte(current.Function.Arguments), &currentArgs)
	} else if current.Args != nil {
		currentArgs = current.Args
	}

	// получаем путь к файлу из текущего вызова
	currentPath := p.extractPathFromArgs(currentArgs)
	currentToolName := current.Function.Name
	if currentToolName == "" {
		currentToolName = current.Name
	}

	// проверяем зависимости для каждого предыдущего инструмента
	for i, prev := range previous {
		var prevArgs map[string]any
		if prev.Function.Arguments != "" {
			json.Unmarshal([]byte(prev.Function.Arguments), &prevArgs)
		} else if prev.Args != nil {
			prevArgs = prev.Args
		}

		prevPath := p.extractPathFromArgs(prevArgs)
		prevToolName := prev.Function.Name
		if prevToolName == "" {
			prevToolName = prev.Name
		}

		// зависимости для операций с файлами
		if currentPath != "" && prevPath != "" {
			// apply_diff зависит от write_file или read_file на том же файле
			if currentToolName == "apply_diff" && currentPath == prevPath {
				if prevToolName == "write_file" || prevToolName == "read_file" {
					deps = append(deps, fmt.Sprintf("step_%d", i+1))
				}
			}

			// read_file после write_file на том же файле
			if currentToolName == "read_file" && prevToolName == "write_file" && currentPath == prevPath {
				deps = append(deps, fmt.Sprintf("step_%d", i+1))
			}

			// любая операция с файлом зависит от make_dir для родительской директории
			if prevToolName == "make_dir" && strings.HasPrefix(currentPath, prevPath) {
				deps = append(deps, fmt.Sprintf("step_%d", i+1))
			}
		}

		// тестирование и валидация после модификации
		switch currentToolName {
		case "go_test", "go_vet", "validate_files":
			if prevToolName == "write_file" || prevToolName == "apply_diff" {
				deps = append(deps, fmt.Sprintf("step_%d", i+1))
			}

		case "analyze_code_go":
			// анализ после записи или чтения файла
			if currentPath == prevPath && (prevToolName == "write_file" || prevToolName == "read_file") {
				deps = append(deps, fmt.Sprintf("step_%d", i+1))
			}

		case "attempt_completion":
			// завершение зависит от всех операций анализа и модификации
			if isAnalysisOrModificationTool(prevToolName) {
				deps = append(deps, fmt.Sprintf("step_%d", i+1))
			}
		}
	}

	return deps
}

// extractPathFromArgs извлекает путь к файлу из аргументов инструмента
func (p *Planner) extractPathFromArgs(args map[string]any) string {
	// проверяем различные варианты названий параметра пути
	pathKeys := []string{"path", "file", "fileName", "file_path", "target"}
	for _, key := range pathKeys {
		if path, ok := args[key].(string); ok && path != "" {
			return path
		}
	}
	return ""
}

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

		parallelGroup := []string{step.ID}
		processed[step.ID] = true

		for _, otherStep := range plan.Steps {
			if processed[otherStep.ID] {
				continue
			}

			if p.canRunInParallel(step, otherStep, plan.Steps) {
				parallelGroup = append(parallelGroup, otherStep.ID)
				processed[otherStep.ID] = true
			}
		}

		parallelGroups = append(parallelGroups, parallelGroup)
	}

	return parallelGroups
}

func (p *Planner) canRunInParallel(step1, step2 *types.ExecutionStep, allSteps []*types.ExecutionStep) bool {
	if p.hasDependency(step1, step2, allSteps) || p.hasDependency(step2, step1, allSteps) {
		return false
	}

	if p.conflictingResources(step1, step2) {
		return false
	}

	if p.isSequentialTool(step1.ToolName) || p.isSequentialTool(step2.ToolName) {
		return false
	}

	return true
}

func (p *Planner) hasDependency(step1, step2 *types.ExecutionStep, allSteps []*types.ExecutionStep) bool {
	for _, dep := range step1.Dependencies {
		if dep == step2.ID {
			return true
		}
	}

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

func (p *Planner) conflictingResources(step1, step2 *types.ExecutionStep) bool {
	path1 := p.getFilePath(step1)
	path2 := p.getFilePath(step2)

	if path1 != "" && path2 != "" && path1 == path2 {
		if p.isWriteOperation(step1.ToolName) && p.isWriteOperation(step2.ToolName) {
			return true
		}
	}

	return false
}

func (p *Planner) getFilePath(step *types.ExecutionStep) string {
	if path, ok := step.Args["path"].(string); ok {
		return path
	}
	if file, ok := step.Args["file"].(string); ok {
		return file
	}
	return ""
}

func (p *Planner) isWriteOperation(toolName string) bool {
	writeTools := map[string]bool{
		"write_file": true,
		"apply_diff": true,
	}
	return writeTools[toolName]
}

func (p *Planner) isSequentialTool(toolName string) bool {
	sequentialTools := map[string]bool{
		"attempt_completion": true,
		"execute_command":    true,
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

		allDepsCompleted := true
		for _, depID := range step.Dependencies {
			depStep := p.findStepByID(plan, depID)
			if depStep == nil {
				// логируем отсутствующую зависимость
				fmt.Printf("WARNING: Dependency %s not found for step %s (%s)\n",
					depID, step.ID, step.ToolName)
				allDepsCompleted = false
				break
			}
			if depStep.Status != types.StepStatusCompleted {
				// логируем незавершённую зависимость
				if depStep.Status == types.StepStatusFailed {
					fmt.Printf("WARNING: Dependency %s failed for step %s (%s)\n",
						depID, step.ID, step.ToolName)
				}
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

func (p *Planner) extractFilesAffected(call entity.ToolCall) []string {
	var files []string

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
