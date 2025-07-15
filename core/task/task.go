package task

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"autonomy/core/entity"
	"autonomy/core/tools"
	"autonomy/ui"
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
		MaxHistorySize:    20,
		AICallTimeout:     100 * time.Second,
		ToolTimeout:       30 * time.Second,
		MinAPIInterval:    time.Second,
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

		if done, err := t.executeTools(response.ToolCalls); err != nil {
			fmt.Println(ui.Error("Tool execution error: " + err.Error()))
			
			continue
		} else if done {
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

	// Thread-safe prompt copy
	t.mu.RLock()
	promptCopy := t.copyPromptData()
	t.mu.RUnlock()

	response, err := t.client.GenerateCode(ctx, promptCopy)
	if err != nil {
		return nil, err
	}

	log.Printf("AI response: %s", response.Content)
	return response, nil
}

// executeTools runs tool calls sequentially
func (t *Task) executeTools(calls []entity.ToolCall) (bool, error) {
	fmt.Println(ui.Tool(fmt.Sprintf("Executing %d tools...", len(calls))))

	ctx, cancel := context.WithTimeout(t.ctx, 5*time.Minute)
	defer cancel()

	for i, call := range calls {
		if err := t.checkContext(ctx); err != nil {
			return false, err
		}

		fmt.Printf("%s\n", ui.Blue(fmt.Sprintf("📋 Tool %d/%d: %s", i+1, len(calls), call.Name)))

		result, err := t.exec(ctx, call)
		t.handleToolResult(call, result, err)

		if call.Name == "attempt_completion" && err == nil {
			return true, nil
		}
	}

	return false, nil
}

// exec executes a single tool
func (t *Task) exec(ctx context.Context, call entity.ToolCall) (string, error) {
	toolCtx, cancel := context.WithTimeout(ctx, t.config.ToolTimeout)
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
		return "", fmt.Errorf("tool %s timed out after %v", call.Name, t.config.ToolTimeout)
	}
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

	// Success path
	if isSilentTool(call.Name) {
		summary := silentToolSummary(call.Name, call.Args, result)
		fmt.Println(ui.Success("Done "+call.Name) + summary)
	} else {
		fmt.Println(ui.Success("Done " + call.Name))
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
		return fmt.Errorf("task cancelled: %v", t.ctx.Err())
	default:
		return nil
	}
}

func (t *Task) checkContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("operation timed out or was cancelled")
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
		cut := t.config.MaxHistorySize / 2
		excess := len(t.promptData.Messages) - t.config.MaxHistorySize

		if cut < 1 {
			cut = 1
		}

		if excess < cut {
			cut = excess
		}

		t.promptData.Messages = append(
			t.promptData.Messages[:1],
			t.promptData.Messages[cut+1:]...,
		)
		log.Printf("Trimmed %d old messages from history", cut)
	}
}

func (t *Task) handleNoTools() bool {
	t.mu.Lock()
	t.noToolCount++
	count := t.noToolCount
	t.mu.Unlock()

	if count >= t.config.MaxNoToolAttempts {
		fmt.Println(ui.Warning("AI failed to start the task (no tool invocations). Aborting."))

		return true
	}

	fmt.Println(ui.Warning("AI did not use any tools. Forcing tool usage..."))
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
	defer t.mu.Unlock()

	elapsed := time.Since(t.lastAPICall)
	if elapsed < t.config.MinAPIInterval {
		waitTime := t.config.MinAPIInterval - elapsed
		t.mu.Unlock()

		select {
		case <-time.After(waitTime):
			t.mu.Lock()
			t.lastAPICall = time.Now()
			return nil
		case <-t.ctx.Done():
			return t.ctx.Err()
		}
	}

	t.lastAPICall = time.Now()
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
