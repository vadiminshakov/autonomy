package task

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

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

// defaultConfig returns default task configuration
func defaultConfig() Config {
	return Config{
		MaxIterations:        100,
		MaxHistorySize:       80,
		AICallTimeout:        300 * time.Second,
		ToolTimeout:          20 * time.Second,
		MaxNoToolAttempts:    10,
		EnableReflection:     true,
		EnableFileValidation: true,
	}
}

// AIClient abstracts AI model client
//
//go:generate mockgen -destination ../../mocks/ai_client_mock.go -package mocks autonomy/core/task AIClient
type AIClient interface {
	GenerateCode(ctx context.Context, promptData entity.PromptData) (*entity.AIResponse, error)
}

// Task manages AI-driven task execution
type Task struct {
	client     AIClient
	promptData *entity.PromptData
	config     Config

	mu                sync.RWMutex
	ctx               context.Context
	cancel            context.CancelFunc
	noToolCount       int
	justExecutedTools bool

	planner          *Planner
	reflectionEngine *reflection.ReflectionEngine
	originalTask     string
}

// NewTask creates a new task with default configuration
func NewTask(client AIClient) *Task {
	return NewTaskWithConfig(client, defaultConfig())
}

// NewTaskWithConfig creates a new task with custom configuration
func NewTaskWithConfig(client AIClient, config Config) *Task {
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

		log.Printf("=== Task iteration %d/%d ===", iter+1, t.config.MaxIterations)

		response, err := t.callAi()
		if err != nil {
			return t.handleAIError(err)
		}

		// add assistant message with tool calls if present
		if len(response.ToolCalls) > 0 {
			t.addAssistantMessageWithTools(response.Content, response.ToolCalls)
		} else {
			t.addAssistantMessage(response.Content)
		}

		if len(response.ToolCalls) == 0 {
			// don't count as "no tools" if we just executed tools
			t.mu.Lock()
			justExecuted := t.justExecutedTools
			if justExecuted {
				t.justExecutedTools = false
				log.Printf("Skipping no-tools handling because tools were just executed")
			}
			t.mu.Unlock()

			if justExecuted {
				continue
			}
			if shouldAbort := t.handleNoTools(); shouldAbort {
				// instead of returning an error, try one final attempt to get completion
				log.Printf("Final attempt to get completion after tool selection failure")
				finalResponse, err := t.callAi()
				if err == nil && len(finalResponse.ToolCalls) > 0 {
					// if we got tools this time, execute them
					done, err := t.executeTools(finalResponse.ToolCalls)
					if err == nil && done {
						return nil
					}
				}
				// if still no tools or completion, finish gracefully
				fmt.Println(ui.Info("Task completed with partial results due to tool selection difficulties."))
				return nil
			}
			continue
		}

		var toolNames []string
		for _, call := range response.ToolCalls {
			toolNames = append(toolNames, call.Name)
		}
		log.Printf("AI requested tools: %s", strings.Join(toolNames, ", "))

		t.resetNoToolCount()

		done, err := t.executeTools(response.ToolCalls)
		if err != nil {
			log.Printf("Tool execution error: %v", err)
			continue
		}

		// mark that we just executed tools
		t.mu.Lock()
		t.justExecutedTools = true
		log.Printf("Tools executed successfully, setting justExecutedTools flag")
		t.mu.Unlock()

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

// callAi gets response from AI with rate limiting
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

// executeTools runs tool calls
func (t *Task) executeTools(calls []entity.ToolCall) (bool, error) {
	if plan := t.getPlanFromDecompositionIfExists(); plan != nil {
		return t.executePlan(plan)
	}

	if t.shouldUsePlanning(calls) {
		plan := t.planner.CreatePlan(calls)
		return t.executePlan(plan)
	}

	// fall back to sequential execution for simple tasks
	return t.executeSequential(calls)
}

// getPlanFromDecompositionIfExists retrieves a stored execution plan if available
func (t *Task) getPlanFromDecompositionIfExists() *types.ExecutionPlan {
	if !hasDecomposedTask() {
		return nil
	}

	decomposedTask, err := getDecomposedTask()
	if err != nil {
		log.Printf("Error retrieving decomposed task: %v", err)
	}

	toolCalls := decomposedTask.ConvertToToolCalls()
	plan := t.planner.CreatePlan(toolCalls)
	clearDecomposedTask()

	log.Printf("Created execution plan from decomposed task: %s", decomposedTask.OriginalTask)
	fmt.Print(ui.Dim(decomposedTask.GetStepSummary()))
	return plan
}

// shouldUsePlanning determines if a task should use planning based on complexity
func (t *Task) shouldUsePlanning(calls []entity.ToolCall) bool {
	// use planning for tasks with multiple tools
	if len(calls) >= 2 {
		return true
	}

	// use planning for tasks that involve file analysis and modification
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

// executePlan executes an execution plan using parallel execution
func (t *Task) executePlan(plan *types.ExecutionPlan) (bool, error) {
	fmt.Println(ui.Tool(fmt.Sprintf("Executing plan with %d steps...", len(plan.Steps))))
	startTime := time.Now()

	executor := NewParallelExecutor(4, 5*time.Minute)
	err := executor.ExecutePlan(t.ctx, plan)
	if err != nil {
		return false, err
	}

	t.addPlanResultsToHistory(plan)

	// perform file validation if enabled
	if t.config.EnableFileValidation {
		validationStartTime := time.Now()
		validationResults, err := tools.Execute("validate_modified_files", map[string]interface{}{})
		if err != nil {
			log.Printf("File validation error: %v", err)
		} else {
			validationTime := time.Since(validationStartTime)
			log.Printf("File validation completed in %v", validationTime)
			if validationResults != "" {
				log.Printf("Validation results:\n%s", validationResults)
			}
		}
	}

	// perform reflection if enabled
	if t.config.EnableReflection && t.reflectionEngine != nil && t.originalTask != "" {
		executionTime := time.Since(startTime)
		reflection, err := t.reflectionEngine.EvaluateCompletion(t.ctx, plan, t.originalTask)
		if err != nil {
			log.Printf("Reflection error: %v", err)
		} else {
			log.Printf("Reflection completed in %v", executionTime)
			t.handleReflectionResult(reflection)
		}
	}

	return t.checkCompletion(plan), nil
}

// executeSequential runs tool calls sequentially (fallback)
func (t *Task) executeSequential(calls []entity.ToolCall) (bool, error) {
	ctx, cancel := context.WithTimeout(t.ctx, 5*time.Minute)
	defer cancel()

	for _, call := range calls {
		if err := t.checkContext(ctx); err != nil {
			return false, err
		}

		fmt.Printf("%s\n", ui.Blue(fmt.Sprintf("📋 Tool %s", call.Name)))

		spinner := ui.ShowToolExecution(call.Name)

		result, err := t.exec(ctx, call)

		spinner.Stop()
		t.handleToolResult(call, result, err)

		if call.Name == "attempt_completion" && err == nil {
			return true, nil
		}
	}

	return false, nil
}

// checkCompletion checks if the execution plan indicates task completion
func (t *Task) checkCompletion(plan *types.ExecutionPlan) bool {
	for _, step := range plan.Steps {
		if step.ToolName == "attempt_completion" && step.Status == types.StepStatusCompleted {
			return true
		}
	}
	return false
}

// exec executes a single tool
func (t *Task) exec(ctx context.Context, call entity.ToolCall) (string, error) {
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

// getToolTimeout returns appropriate timeout for different tool types
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

// handleToolResult processes tool execution result
func (t *Task) handleToolResult(call entity.ToolCall, result string, err error) {
	if err != nil {
		fmt.Println(ui.Error(fmt.Sprintf("Error running %s: %v", call.Name, err)))
		if result != "" && !isSilentTool(call.Name) {
			fmt.Printf("%s\n", ui.Dim("Result: "+result))
		}
		t.addUserMessage(fmt.Sprintf("Error executing %s: %v. Result: %s", call.Name, err, result))

		return
	}

	if isSilentTool(call.Name) {
		summary := silentToolSummary(call.Name, call.Args, result)
		fmt.Println(ui.Success("Done "+call.Name) + summary)
	} else {
		fmt.Println(ui.Success("Done " + call.Name))

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
				fmt.Printf("%s\n", ui.Dim("Result: "+result))
			}
		}
	}

	// add tool result as tool response message for AI context
	t.addToolResponse(call.Name, result)
}

// limitToolOutput truncates tool output if it's too verbose
func limitToolOutput(result string) string {
	maxLines := 10
	lines := strings.Split(result, "\n")
	if len(lines) > maxLines {
		truncated := strings.Join(lines[:maxLines], "\n")
		return truncated + "\n..."
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

func (t *Task) addAssistantMessageWithTools(content string, toolCalls []entity.ToolCall) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.promptData.AddAssistantMessageWithTools(content, toolCalls)
	t.trimHistoryIfNeeded()
}

func (t *Task) addUserMessage(message string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.promptData.AddMessage("user", message)
	t.trimHistoryIfNeeded()
}

func (t *Task) addToolResponse(toolName, result string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// generate a consistent tool call ID based on tool name
	toolCallID := fmt.Sprintf("call_%s", toolName)
	t.promptData.AddToolResponse(toolCallID, result)
	t.trimHistoryIfNeeded()
}

func (t *Task) trimHistoryIfNeeded() {
	if len(t.promptData.Messages) > t.config.MaxHistorySize {
		// сut half of MaxHistorySize from the start, but always keep the first message
		cut := len(t.promptData.Messages) / 2

		if cut < 1 {
			cut = 1
		}

		contextMsg := t.contextCompaction(cut)

		if contextMsg != "" {
			// insert context message after the first message
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

		log.Printf("Trimmed %d old messages from history", cut)
	}
}

// contextCompaction creates a summary of important context from messages being trimmed
func (t *Task) contextCompaction(messagesToTrim int) string {
	// extract key information from messages that will be trimmed
	var toolsUsed []string
	var filesModified []string
	seenTools := make(map[string]bool)
	seenFiles := make(map[string]bool)

	// analyze messages that will be trimmed (skip first message)
	for i := 1; i <= messagesToTrim && i < len(t.promptData.Messages); i++ {
		msg := t.promptData.Messages[i]
		content := strings.ToLower(msg.Content)

		// extract tool usage
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

		// extract file operations
		if strings.Contains(content, "write_file") || strings.Contains(content, "apply_diff") {
			// try to extract filename
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

	// build context message
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

	// progressive help strategy based on attempt count
	switch {
	case count <= 3:
		// first few attempts: gentle reminder
		fmt.Println(ui.Warning(fmt.Sprintf("⚠️  AI not using tools (attempt %d/%d). Providing gentle reminder...", count, t.config.MaxNoToolAttempts)))
		t.forceToolUsage()
		return false

	case count <= 6:
		// middle attempts: provide specific guidance
		fmt.Println(ui.Warning(fmt.Sprintf("⚠️  AI struggling with tool selection (attempt %d/%d). Providing guidance...", count, t.config.MaxNoToolAttempts)))
		t.provideToolGuidance()
		return false

	case count < t.config.MaxNoToolAttempts:
		// near limit: try completion
		fmt.Println(ui.Warning(fmt.Sprintf("⚠️  AI struggling with tool selection (attempt %d/%d). Suggesting completion...", count, t.config.MaxNoToolAttempts)))
		t.suggestCompletion()
		return false

	default:
		// final attempt: force completion and gracefully finish
		fmt.Println(ui.Warning("⚠️  AI unable to use tools. Attempting graceful completion..."))
		t.forceCompletion()
		return true // This will end the task, but gracefully
	}
}

func (t *Task) resetNoToolCount() {
	t.mu.Lock()
	t.noToolCount = 0
	t.mu.Unlock()
}

func (t *Task) forceToolUsage() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.promptData.AddMessage("user", t.promptData.GetForceToolsMessage())
}

func (t *Task) provideToolGuidance() {
	t.mu.Lock()
	defer t.mu.Unlock()

	// find the last assistant message to avoid referencing guidance messages
	recentContent := ""
	for i := len(t.promptData.Messages) - 1; i >= 0; i-- {
		msg := t.promptData.Messages[i]
		if msg.Role == "assistant" {
			if len(msg.Content) > 100 {
				recentContent = msg.Content[:100] + "..."
			} else {
				recentContent = msg.Content
			}
			break
		}
	}

	guidance := fmt.Sprintf(`You seem to be having trouble selecting the right tool. Here's some guidance:

Based on your recent response: "%s"

Common tool selection patterns:
- If you need to explore the project: use get_project_structure
- If you need to read a specific file: use view with the file_path
- If you need to search for something: use search_dir or find_files
- If you need to run commands: use bash with the command
- If you need to write/modify files: use write or multiedit
- If you're done with the task: use attempt_completion

Please select ONE appropriate tool and execute it now.`, recentContent)

	t.promptData.AddMessage("user", guidance)
}

func (t *Task) suggestCompletion() {
	t.mu.Lock()
	defer t.mu.Unlock()

	message := `It seems you might be finished with the task. If you have completed what was requested, please use the attempt_completion tool to summarize what was accomplished.

If you still need to do something, please specify exactly what tool you need to use next.

Choose one:
1. Use attempt_completion if the task is done
2. Use a specific tool if more work is needed`

	t.promptData.AddMessage("user", message)
}

func (t *Task) forceCompletion() {
	t.mu.Lock()
	defer t.mu.Unlock()

	// create a fallback completion message
	completion := "Task execution completed. The AI was unable to select appropriate tools after multiple attempts, but the conversation history shows the task progress."

	// add this as a user message requesting completion
	t.promptData.AddMessage("user", fmt.Sprintf("Please use attempt_completion tool with this summary: %s", completion))
}

func (t *Task) copyPromptData() entity.PromptData {
	promptCopy := *t.promptData
	promptCopy.Messages = make([]entity.Message, len(t.promptData.Messages))
	copy(promptCopy.Messages, t.promptData.Messages)
	return promptCopy
}

func (t *Task) handleAIError(err error) error {
	fmt.Println(ui.Error("AI error: " + err.Error()))
	fmt.Println(ui.Info("Please try again or rephrase the request"))

	return err
}

// formatFindFilesArgs formats arguments for find_files tool display
func formatFindFilesArgs(args map[string]any) string {
	var parts []string

	// extract path argument
	if path, ok := args["path"].(string); ok && path != "" {
		if path == "." {
			parts = append(parts, "path: current directory")
		} else {
			parts = append(parts, fmt.Sprintf("path: %s", path))
		}
	}

	// extract pattern argument
	if pattern, ok := args["pattern"].(string); ok && pattern != "" {
		parts = append(parts, fmt.Sprintf("pattern: %s", pattern))
	}

	// extract case_insensitive argument
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

// addPlanResultsToHistory adds execution results from completed plan steps to AI conversation
func (t *Task) addPlanResultsToHistory(plan *types.ExecutionPlan) {
	plan.Mu.RLock()
	defer plan.Mu.RUnlock()

	for _, step := range plan.Steps {
		if step.Status == types.StepStatusCompleted || step.Status == types.StepStatusFailed {
			call := entity.ToolCall{
				Name: step.ToolName,
				Args: step.Args,
			}
			t.handleToolResult(call, step.Result, step.Error)
		}
	}
}

// handleReflectionResult processes reflection evaluation and provides feedback
func (t *Task) handleReflectionResult(reflection *reflection.ReflectionResult) {
	if reflection.TaskCompleted {
		fmt.Println(ui.Success("✅ Reflection: Task completed successfully"))
		fmt.Printf("%s\n", ui.Info("Reason: "+reflection.Reason))
	} else {
		fmt.Println(ui.Warning("🤔 Reflection: Task may not be fully completed"))
		fmt.Printf("%s\n", ui.Info("Reason: "+reflection.Reason))

		if reflection.ShouldRetry {
			fmt.Println(ui.Info("💡 Suggestion: Consider continuing or retrying the task"))
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
