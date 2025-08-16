package task

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/vadiminshakov/autonomy/core/ai"
	"github.com/vadiminshakov/autonomy/core/decomposition"
	"github.com/vadiminshakov/autonomy/core/entity"
	"github.com/vadiminshakov/autonomy/core/reflection"
	"github.com/vadiminshakov/autonomy/core/tools"
	"github.com/vadiminshakov/autonomy/core/types"
	"github.com/vadiminshakov/autonomy/ui"
)

// Config holds task execution configuration
type Config struct {
	MaxIterations        int
	MaxHistorySize       int
	AICallTimeout        time.Duration
	ToolTimeout          time.Duration
	MinAPIInterval       time.Duration
	MaxNoToolAttempts    int
	EnableReflection     bool
	EnableFileValidation bool
}

func defaultConfig() Config {
	return Config{
		MaxIterations:        100,
		MaxHistorySize:       80,
		AICallTimeout:        300 * time.Second,
		ToolTimeout:          30 * time.Second,
		MinAPIInterval:       1 * time.Second,
		MaxNoToolAttempts:    5,
		EnableReflection:     true,
		EnableFileValidation: true,
	}
}

// Task manages AI-driven task execution
type Task struct {
	client     ai.AIClient
	promptData *entity.PromptData
	config     Config

	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	noToolCount int

	planner          *Planner
	reflectionEngine *reflection.ReflectionEngine
	originalTask     string

	toolCallHistory map[string]int
}

// NewTask creates a new task with default configuration
func NewTask(client ai.AIClient) *Task {
	return NewTaskWithConfig(client, defaultConfig())
}

// NewTaskWithConfig creates a new task with custom configuration
func NewTaskWithConfig(client ai.AIClient, config Config) *Task {
	ctx, cancel := context.WithCancel(context.Background())
	var reflectionEngine *reflection.ReflectionEngine
	if config.EnableReflection {
		reflectionEngine = reflection.NewReflectionEngine(client)
	}

	return &Task{
		client:           client,
		promptData:       NewPromptData(),
		config:           config,
		ctx:              ctx,
		cancel:           cancel,
		planner:          NewPlanner(),
		reflectionEngine: reflectionEngine,

		toolCallHistory: make(map[string]int),
	}
}

// SetOriginalTask sets the original task description for reflection
func (t *Task) SetOriginalTask(task string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.originalTask = task
}

// Close releases task resources
func (t *Task) Close() {
	if t.cancel != nil {
		t.cancel()
	}
}

// AddUserMessage adds a user message to history
func (t *Task) AddUserMessage(message string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.promptData.AddMessage("user", message)
	t.trimHistoryIfNeeded()
}

// ProcessTask executes the main task loop
func (t *Task) ProcessTask() error {
	defer t.Close()

	for iter := 0; iter < t.config.MaxIterations; iter++ {
		if err := t.checkCancellation(); err != nil {
			return err
		}

		response, err := t.callAi()
		if err != nil {
			return t.handleAIError(err)
		}

		if response.Content != "" {
			fmt.Printf("AI: %s\n", response.Content)
		}

		if len(response.ToolCalls) > 0 {
			for _, call := range response.ToolCalls {
				argsStr := ""
				if len(call.Args) > 0 {
					args := make([]string, 0, len(call.Args))
					for k, v := range call.Args {
						if k != "_enhanced_metadata" {
							args = append(args, fmt.Sprintf("%s: %v", k, v))
						}
					}
					if len(args) > 0 {
						argsStr = " (" + strings.Join(args, ", ") + ")"
					}
				}
				fmt.Printf("Tool: %s%s\n", call.Name, argsStr)

				if t.isToolCallLoop(call) {
					fmt.Printf("Warning: Tool %s called too many times. This may indicate a loop.\n", call.Name)
				}
			}
		}

		if len(response.ToolCalls) == 0 {
			t.addAssistantMessage(response.Content)
			if shouldAbort := t.handleNoTools(); shouldAbort {
				return errors.New("ai did not provide tool invocations after multiple attempts")
			}
			continue
		}

		t.promptData.AddAssistantMessageWithTools(response.Content, response.ToolCalls)

		var toolNames []string
		for _, call := range response.ToolCalls {
			toolNames = append(toolNames, call.Name)
		}

		t.resetNoToolCount()

		done, err := t.executeTools(response.ToolCalls)
		if err != nil {
			continue
		}

		if done {
			return nil
		}
	}

	fmt.Println(ui.Warning(fmt.Sprintf(
		"Reached limit of %d attempts. Type 'continue' to resume",
		t.config.MaxIterations,
	)))
	return nil
}

func (t *Task) callAi() (*entity.AIResponse, error) {
	ctx, cancel := context.WithTimeout(t.ctx, t.config.AICallTimeout)
	defer cancel()

	t.mu.RLock()
	promptCopy := t.copyPromptData()
	t.mu.RUnlock()

	spinner := ui.ShowThinking()
	defer spinner.Stop()

	response, err := t.client.GenerateCode(ctx, promptCopy)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (t *Task) executeTools(calls []entity.ToolCall) (bool, error) {
	// сначала проверяем, есть ли декомпозированная задача
	if plan := t.getPlanFromDecompositionIfExists(); plan != nil {
		return t.executePlan(plan)
	}

	// простая логика: если больше одного инструмента или есть зависимости - используем план
	if len(calls) > 1 || t.hasComplexDependencies(calls) {
		plan := t.planner.CreatePlan(calls)
		return t.executePlan(plan)
	}

	// иначе выполняем последовательно
	return t.executeSequential(calls)
}

func (t *Task) getPlanFromDecompositionIfExists() *types.ExecutionPlan {
	if !hasDecomposedTask() {
		return nil
	}

	decomposedTask, err := getDecomposedTask()
	if err != nil {
		return nil
	}

	toolCalls := decomposedTask.ConvertToToolCalls()
	plan := t.planner.CreatePlan(toolCalls)
	clearDecomposedTask()

	fmt.Print(ui.Dim(decomposedTask.GetStepSummary()))
	return plan
}

func (t *Task) shouldUsePlanning(calls []entity.ToolCall) bool {
	if len(calls) >= 2 {
		return true
	}

	hasAnalysis := false
	hasModification := false

	for _, call := range calls {
		switch call.Name {
		case "read_file", "analyze_code_go", "search_dir", "search_index":
			hasAnalysis = true
		case "write_file", "apply_diff":
			hasModification = true
		}
	}

	return hasAnalysis && hasModification
}

func (t *Task) executePlan(plan *types.ExecutionPlan) (bool, error) {
	fmt.Printf("Executing plan with %d steps...\n", len(plan.Steps))

	// используем параллельный исполнитель
	executor := NewParallelExecutor(4, 5*time.Minute)

	// выполняем план
	err := executor.ExecutePlan(t.ctx, plan)
	if err != nil {
		fmt.Printf("Plan execution error: %v\n", err)
		return false, err
	}

	// добавляем результаты в историю
	t.addPlanResultsToHistory(plan)

	// проверяем завершение
	return t.checkCompletion(plan), nil
}

func (t *Task) executeSequential(calls []entity.ToolCall) (bool, error) {
	fmt.Printf("Executing %d tools sequentially...\n", len(calls))

	ctx, cancel := context.WithTimeout(t.ctx, 5*time.Minute)
	defer cancel()

	for i, call := range calls {
		if err := t.checkContext(ctx); err != nil {
			return false, err
		}

		fmt.Printf("Tool %d/%d: %s\n", i+1, len(calls), call.Name)

		// выполняем инструмент
		result, err := t.exec(ctx, call)

		// обрабатываем результат
		t.handleToolResult(call, result, err)

		// проверяем завершение
		if call.Name == "attempt_completion" && err == nil {
			return true, nil
		}
	}

	return false, nil
}

// hasComplexDependencies проверяет, есть ли сложные зависимости между инструментами
func (t *Task) hasComplexDependencies(calls []entity.ToolCall) bool {
	// проверяем, есть ли операции чтения и записи одних и тех же файлов
	fileOps := make(map[string][]string)

	for _, call := range calls {
		var args map[string]any
		if call.Args != nil {
			args = call.Args
		}

		// извлекаем путь к файлу
		var path string
		for _, key := range []string{"path", "file", "fileName", "file_path"} {
			if p, ok := args[key].(string); ok {
				path = p
				break
			}
		}

		if path != "" {
			fileOps[path] = append(fileOps[path], call.Name)
		}
	}

	// если есть несколько операций с одним файлом - нужен план
	for _, ops := range fileOps {
		if len(ops) > 1 {
			return true
		}
	}

	// проверяем наличие операций, требующих последовательности
	hasAnalysis := false
	hasModification := false

	for _, call := range calls {
		switch call.Name {
		case "read_file", "analyze_code_go", "search_dir", "search_index":
			hasAnalysis = true
		case "write_file", "apply_diff":
			hasModification = true
		case "go_test", "go_vet":
			// тесты всегда требуют зависимостей
			return true
		}
	}

	return hasAnalysis && hasModification
}

func (t *Task) checkCompletion(plan *types.ExecutionPlan) bool {
	for _, step := range plan.Steps {
		if step.ToolName == "attempt_completion" && step.Status == types.StepStatusCompleted {
			return true
		}
	}
	return false
}

func (t *Task) exec(ctx context.Context, call entity.ToolCall) (string, error) {
	// Check if tool is available before execution
	availableTools := tools.List()
	toolExists := false
	for _, tool := range availableTools {
		if tool == call.Name {
			toolExists = true
			break
		}
	}

	if !toolExists {
		return "", fmt.Errorf("tool %s is not available", call.Name)
	}

	timeout := t.getToolTimeout(call.Name)
	toolCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resultChan := make(chan struct {
		res string
		err error
	}, 1)

	go func() {
		res, err := tools.Execute(call.Name, call.Args)
		resultChan <- struct {
			res string
			err error
		}{res, err}
	}()

	select {
	case result := <-resultChan:
		return result.res, result.err
	case <-toolCtx.Done():
		return "", fmt.Errorf("tool %s timed out after %v", call.Name, timeout)
	}
}

func (t *Task) getToolTimeout(toolName string) time.Duration {
	longRunningTools := map[string]time.Duration{
		"decompose_task":  9 * time.Minute,
		"execute_command": 3 * time.Minute,
		"go_test":         2 * time.Minute,
		"search_index":    2 * time.Minute,
		"analyze_code_go": 1 * time.Minute,
	}

	if timeout, ok := longRunningTools[toolName]; ok {
		return timeout
	}

	return t.config.ToolTimeout
}

func (t *Task) handleToolResult(call entity.ToolCall, result string, err error) {
	if err != nil {
		fmt.Println(ui.Error(fmt.Sprintf("Error running %s: %v", call.Name, err)))
		if result != "" && !isSilentTool(call.Name) {
			fmt.Printf("%s\n", ui.Dim("Result: "+result))
		}
		t.promptData.AddToolResponse(call.ID, fmt.Sprintf("Error: %v. Result: %s", err, result))
		return
	}

	if isSilentTool(call.Name) {
		summary := silentToolSummary(call.Name, call.Args, result)
		fmt.Println(ui.Success("✓ "+call.Name) + summary)
	} else {
		fmt.Println(ui.Success("✓ " + call.Name))

		if call.Name == "find_files" {
			argsInfo := formatFindFilesArgs(call.Args)
			if argsInfo != "" {
				fmt.Printf("%s\n", ui.Info("Arguments: "+argsInfo))
			}
		}

		if result != "" {
			if call.Name == "attempt_completion" {
				fmt.Println(ui.Info(result))
			} else {
				result = limitToolOutput(result)
				fmt.Printf("Tool result: %s\n", result)
			}
		}
	}

	t.promptData.AddToolResponse(call.ID, result)
}

func limitToolOutput(result string) string {
	maxLines := 20
	maxChars := 2000

	if len(result) > maxChars {
		result = result[:maxChars] + "\n... [truncated]"
	}

	lines := strings.Split(result, "\n")
	if len(lines) > maxLines {
		truncated := strings.Join(lines[:maxLines], "\n")
		return truncated + "\n... [truncated]"
	}

	return result
}

func (t *Task) checkCancellation() error {
	select {
	case <-t.ctx.Done():
		return fmt.Errorf("task canceled: %v", t.ctx.Err())
	default:
		return nil
	}
}

func (t *Task) checkContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("operation timed out or was canceled")
	default:
		return nil
	}
}

func (t *Task) addAssistantMessage(content string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.promptData.AddMessage("assistant", content)
	t.trimHistoryIfNeeded()
}

func (t *Task) addUserMessage(message string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.promptData.AddMessage("user", message)
	t.trimHistoryIfNeeded()
}

func (t *Task) trimHistoryIfNeeded() {
	if len(t.promptData.Messages) > t.config.MaxHistorySize {
		cut := len(t.promptData.Messages) / 2

		if cut < 1 {
			cut = 1
		}

		contextMsg := t.contextCompaction(cut)

		if contextMsg != "" {
			summaryMsg := entity.Message{
				Role:    "system",
				Content: contextMsg,
			}

			t.promptData.Messages = append(
				[]entity.Message{t.promptData.Messages[0], summaryMsg},
				t.promptData.Messages[cut+1:]...,
			)
		} else {
			t.promptData.Messages = append(
				t.promptData.Messages[:1],
				t.promptData.Messages[cut+1:]...,
			)
		}
	}
}

func (t *Task) contextCompaction(messagesToTrim int) string {
	var toolsUsed []string
	var filesModified []string
	seenTools := make(map[string]bool)
	seenFiles := make(map[string]bool)

	for i := 1; i <= messagesToTrim && i < len(t.promptData.Messages); i++ {
		msg := t.promptData.Messages[i]
		content := strings.ToLower(msg.Content)

		if strings.Contains(content, "result of ") {
			parts := strings.Split(content, ":")
			if len(parts) > 0 {
				toolName := strings.TrimSpace(strings.TrimPrefix(parts[0], "result of "))
				if !seenTools[toolName] {
					toolsUsed = append(toolsUsed, toolName)
					seenTools[toolName] = true
				}
			}
		}

		if strings.Contains(content, "write_file") || strings.Contains(content, "apply_diff") {
			if idx := strings.Index(content, "path:"); idx != -1 {
				pathPart := content[idx+5:]
				if endIdx := strings.IndexAny(pathPart, " \n,}"); endIdx != -1 {
					filename := strings.TrimSpace(pathPart[:endIdx])
					if !seenFiles[filename] {
						filesModified = append(filesModified, filename)
						seenFiles[filename] = true
					}
				}
			}
		}
	}

	if len(toolsUsed) == 0 && len(filesModified) == 0 {
		return ""
	}

	var contextParts []string
	contextParts = append(contextParts, "CONTEXT FROM PREVIOUS MESSAGES:")

	if len(toolsUsed) > 0 {
		contextParts = append(contextParts, fmt.Sprintf("Tools already used: %s", strings.Join(toolsUsed, ", ")))
	}

	if len(filesModified) > 0 {
		contextParts = append(contextParts, fmt.Sprintf("Files modified: %s", strings.Join(filesModified, ", ")))
	}

	contextParts = append(contextParts, "Continue building on this work.")

	return strings.Join(contextParts, "\n")
}

func (t *Task) handleNoTools() bool {
	t.mu.Lock()
	t.noToolCount++
	count := t.noToolCount
	t.mu.Unlock()

	if count >= t.config.MaxNoToolAttempts {
		fmt.Println(ui.Warning("AI failed to use tools after multiple attempts. Aborting."))
		return true
	}

	t.forceToolUsage()

	return false
}

func (t *Task) resetNoToolCount() {
	t.mu.Lock()
	t.noToolCount = 0
	t.mu.Unlock()
}

func (t *Task) isToolCallLoop(call entity.ToolCall) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	key := call.Name
	if call.Args != nil {
		argsStr := fmt.Sprintf("%v", call.Args)
		key += "_" + argsStr
	}

	count := t.toolCallHistory[key]
	t.toolCallHistory[key] = count + 1

	return count >= 3
}

func (t *Task) forceToolUsage() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.promptData.AddMessage("user", t.promptData.GetForceToolsMessage())
}

func (t *Task) copyPromptData() entity.PromptData {
	promptCopy := *t.promptData
	promptCopy.Messages = make([]entity.Message, len(t.promptData.Messages))
	copy(promptCopy.Messages, t.promptData.Messages)
	return promptCopy
}

func (t *Task) handleAIError(err error) error {
	fmt.Println(ui.Error("AI error: " + err.Error()))

	if strings.Contains(err.Error(), "400 Bad Request") {
		fmt.Println(ui.Warning("This usually means:"))
		fmt.Println(ui.Dim("   • The model doesn't support function calling"))
		fmt.Println(ui.Dim("   • The request format is incompatible"))
		fmt.Println(ui.Dim("   • Try using a different model or provider"))
	} else if strings.Contains(err.Error(), "401") {
		fmt.Println(ui.Warning("This usually means:"))
		fmt.Println(ui.Dim("   • API key is invalid or expired"))
		fmt.Println(ui.Dim("   • Check your configuration"))
	} else if strings.Contains(err.Error(), "429") {
		fmt.Println(ui.Warning("This usually means:"))
		fmt.Println(ui.Dim("   • Rate limit exceeded"))
		fmt.Println(ui.Dim("   • Wait a moment and try again"))
	}

	fmt.Println(ui.Info("Please try again or rephrase the request"))

	return err
}

func formatFindFilesArgs(args map[string]any) string {
	var parts []string

	if path, ok := args["path"].(string); ok && path != "" {
		if path == "." {
			parts = append(parts, "path: current directory")
		} else {
			parts = append(parts, fmt.Sprintf("path: %s", path))
		}
	}

	if pattern, ok := args["pattern"].(string); ok && pattern != "" {
		parts = append(parts, fmt.Sprintf("pattern: %s", pattern))
	}

	if caseInsensitive, ok := args["case_insensitive"]; ok {
		var isInsensitive bool
		switch v := caseInsensitive.(type) {
		case bool:
			isInsensitive = v
		case string:
			isInsensitive = (v == "true" || v == "1")
		}
		if isInsensitive {
			parts = append(parts, "case_insensitive: true")
		}
	}

	return strings.Join(parts, ", ")
}

func (t *Task) addPlanResultsToHistory(plan *types.ExecutionPlan) {
	plan.Mu.RLock()
	defer plan.Mu.RUnlock()

	for _, step := range plan.Steps {
		if step.Status == types.StepStatusCompleted || step.Status == types.StepStatusFailed {
			call := entity.ToolCall{
				ID:   fmt.Sprintf("plan_%s", step.ID),
				Name: step.ToolName,
				Args: step.Args,
			}
			t.handleToolResult(call, step.Result, step.Error)
		}
	}
}

func (t *Task) handleReflectionResult(reflection *reflection.ReflectionResult) {
	if reflection.TaskCompleted {
		fmt.Println(ui.Success("Reflection: Task completed successfully"))
		fmt.Printf("%s\n", ui.Info("Reason: "+reflection.Reason))
	} else {
		fmt.Println(ui.Warning("Reflection: Task may not be fully completed"))
		fmt.Printf("%s\n", ui.Info("Reason: "+reflection.Reason))

		if reflection.ShouldRetry {
			fmt.Println(ui.Info("Suggestion: Consider continuing or retrying the task"))
			t.addUserMessage("The reflection system suggests the task is not fully complete. Reason: " +
				reflection.Reason + ". Please review and continue if needed.")
		}
	}
}

func hasDecomposedTask() bool {
	state := tools.GetTaskState()
	hasTask, exists := state.GetContext("has_decomposed_task")
	return exists && hasTask == true
}

func getDecomposedTask() (*decomposition.DecompositionResult, error) {
	state := tools.GetTaskState()

	hasTask, exists := state.GetContext("has_decomposed_task")
	if !exists || hasTask != true {
		return nil, fmt.Errorf("no decomposed task found")
	}

	taskData, exists := state.GetContext("decomposed_task")
	if !exists {
		return nil, fmt.Errorf("decomposed task flag set but no task data found")
	}

	result, ok := taskData.(*decomposition.DecompositionResult)
	if !ok {
		return nil, fmt.Errorf("invalid decomposed task data type")
	}

	return result, nil
}

func clearDecomposedTask() {
	state := tools.GetTaskState()
	state.SetContext("has_decomposed_task", false)
	state.SetContext("decomposed_task", nil)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
