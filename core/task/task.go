package task

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/vadiminshakov/autonomy/core/entity"
	"github.com/vadiminshakov/autonomy/core/tools"
	"github.com/vadiminshakov/autonomy/ui"
)

// Config holds task execution configuration
type Config struct {
	MaxIterations     int
	MaxHistorySize    int
	AICallTimeout     time.Duration
	ToolTimeout       time.Duration
	MinAPIInterval    time.Duration
	MaxNoToolAttempts int
}

// DefaultConfig returns default task configuration
func defaultConfig() Config {
	return Config{
		MaxIterations:     100,
		MaxHistorySize:    80,
		AICallTimeout:     100 * time.Second,
		ToolTimeout:       30 * time.Second,
		MinAPIInterval:    3 * time.Second,
		MaxNoToolAttempts: 3,
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

	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	noToolCount int
	lastAPICall time.Time

	// Planning components
	planner *Planner
}

// NewTask creates a new task with default configuration
func NewTask(client AIClient) *Task {
	return NewTaskWithConfig(client, defaultConfig())
}

// NewTaskWithConfig creates a new task with custom configuration
func NewTaskWithConfig(client AIClient, config Config) *Task {
	ctx, cancel := context.WithCancel(context.Background())
	return &Task{
		client:     client,
		promptData: NewPromptData(),
		config:     config,
		ctx:        ctx,
		cancel:     cancel,
		planner:    NewPlanner(),
	}
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

		t.addAssistantMessage(response.Content)

		if len(response.ToolCalls) == 0 {
			if shouldAbort := t.handleNoTools(); shouldAbort {
				return errors.New("ai did not provide tool invocations after multiple attempts")
			}
			continue
		}

		t.resetNoToolCount()

		done, err := t.executeTools(response.ToolCalls)
		if err != nil {
			log.Printf("Tool execution error: %v", err)
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

// callAi gets response from AI with rate limiting
func (t *Task) callAi() (*entity.AIResponse, error) {
	if err := t.enforceRateLimit(); err != nil {
		return nil, fmt.Errorf("rate limit error: %v", err)
	}

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
	// check if we have a stored execution plan from plan_execution tool
	if plan := t.getStoredPlan(); plan != nil {
		return t.executePlan(plan)
	}

	// Check if we should use planning for this set of tool calls
	if t.shouldUsePlanning(calls) {
		plan := t.planner.CreatePlan(calls)
		return t.executePlan(plan)
	}

	// Fall back to sequential execution for simple tasks
	return t.executeSequential(calls)
}

// getStoredPlan retrieves a stored execution plan if available
func (t *Task) getStoredPlan() *ExecutionPlan {
	if tools.HasStoredToolCalls() {
		taskDesc, toolCalls, err := tools.GetStoredToolCalls()
		if err != nil {
			log.Printf("Error retrieving stored tool calls: %v", err)
			return nil
		}

		plan := t.planner.CreatePlan(toolCalls)
		tools.ClearStoredToolCalls()

		log.Printf("Created execution plan from stored tool calls for: %s", taskDesc)
		return plan
	}

	return nil
}

// shouldUsePlanning determines if a task should use planning based on complexity
func (t *Task) shouldUsePlanning(calls []entity.ToolCall) bool {
	// Use planning for tasks with multiple tools
	if len(calls) >= 5 {
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
func (t *Task) executePlan(plan *ExecutionPlan) (bool, error) {
	fmt.Println(ui.Tool(fmt.Sprintf("Executing plan with %d steps...", len(plan.Steps))))

	executor := NewParallelExecutor(4, 5*time.Minute)
	err := executor.ExecutePlan(t.ctx, plan)
	if err != nil {
		log.Printf("Plan execution failed: %v", err)
		return false, err
	}

	return t.checkCompletion(plan), nil
}

// executeSequential runs tool calls sequentially (fallback)
func (t *Task) executeSequential(calls []entity.ToolCall) (bool, error) {
	fmt.Println(ui.Tool(fmt.Sprintf("Executing %d tools sequentially...", len(calls))))

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
func (t *Task) checkCompletion(plan *ExecutionPlan) bool {
	for _, step := range plan.Steps {
		if step.ToolName == "attempt_completion" && step.Status == StepStatusCompleted {
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

	t.addUserMessage(fmt.Sprintf("Result of %s: %s", call.Name, result))
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

// Helper methods
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
		// cut half of MaxHistorySize from the start, but always keep the first message
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

	// check if we've exceeded max attempts
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

func (t *Task) forceToolUsage() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.promptData.AddMessage("user", t.promptData.GetForceToolsMessage())
}

func (t *Task) enforceRateLimit() error {
	t.mu.Lock()
	elapsed := time.Since(t.lastAPICall)

	if elapsed < t.config.MinAPIInterval {
		waitTime := t.config.MinAPIInterval - elapsed
		t.mu.Unlock()

		fmt.Println(ui.Warning(fmt.Sprintf("Rate limit exceeded - waiting %v before next API call", waitTime)))

		select {
		case <-time.After(waitTime):
			t.mu.Lock()
			t.lastAPICall = time.Now()
			t.mu.Unlock()
			return nil
		case <-t.ctx.Done():
			return t.ctx.Err()
		}
	}

	t.lastAPICall = time.Now()
	t.mu.Unlock()
	return nil
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
func formatFindFilesArgs(args map[string]interface{}) string {
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
