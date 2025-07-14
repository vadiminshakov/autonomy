package task

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"autonomy/core/tools"
	"autonomy/ui"
)

const (
	maxIterations      = 100
	maxHistorySize     = 20 // limit conversation history
	aiCallTimeout      = 100 * time.Second
)

// Message represents a single entry in the conversation history.
// Role can be "system", "user", or "assistant".
type Message struct {
	Role    string
	Content string
}

// ToolCall describes a tool invocation extracted from an AI response
type ToolCall struct {
	Name string
	Args map[string]interface{}
}

// ToolResponse describes an AI reply that instructs the agent to execute a tool.
// Tool      – tool name
// Args      – tool arguments
// Message   – free-form assistant message
// Done      – marks the task as finished
type ToolResponse struct {
	Tool    string                 `json:"tool"`
	Args    map[string]interface{} `json:"args"`
	Message string                 `json:"message"`
	Done    bool                   `json:"done"`
}

// AIResponse represents a response from an AI model
type AIResponse struct {
	Content   string     // Text response (if any)
	ToolCalls []ToolCall // Native tool calls (if any)
}

// AIClient abstracts an AI model client implementation.
type AIClient interface {
	GenerateCode(ctx context.Context, promptData *PromptData) (*AIResponse, error)
}

type Task struct {
	client         AIClient
	promptData     *PromptData
	noToolCount    int
	mu             sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
	lastAPICall    time.Time
	minAPIInterval time.Duration
}

// NewTask initializes a fresh Task with the universal system prompt.
func NewTask(client AIClient) *Task {
	ctx, cancel := context.WithCancel(context.Background())
	return &Task{
		client:         client,
		promptData:     NewPromptData(),
		ctx:            ctx,
		cancel:         cancel,
		minAPIInterval: time.Second, // Rate limit: 1 request per second
	}
}

// Close properly shuts down the task and releases resources
func (t *Task) Close() {
	if t.cancel != nil {
		t.cancel()
	}
}

// AddUserMessage appends a user message to the history and logs it.
func (t *Task) AddUserMessage(message string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.promptData.AddMessage("user", message)
	t.trimHistoryIfNeeded()
}

// appendAssistantMessage adds an assistant message to history.
func (t *Task) appendAssistantMessage(content string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.promptData.AddMessage("assistant", content)
	t.trimHistoryIfNeeded()
}

// trimHistoryIfNeeded removes old messages to prevent memory bloat
func (t *Task) trimHistoryIfNeeded() {
	if len(t.promptData.Messages) > maxHistorySize {
		// keep the first message and the last maxHistorySize-1 messages
		excess := len(t.promptData.Messages) - maxHistorySize
		t.promptData.Messages = append(t.promptData.Messages[:1], t.promptData.Messages[excess+1:]...)
		log.Printf("Trimmed %d old messages from history", excess)
	}
}

// forceToolUsage appends a user message that forces the AI to use tools.
func (t *Task) forceToolUsage() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.promptData.AddMessage("user", t.promptData.GetForceToolsMessage())
}

// executeTools runs the parsed tool calls sequentially with timeout and updates conversation history.
// It returns true if the task is completed (attempt_completion).
func (t *Task) executeTools(calls []ToolCall) (bool, error) {
	fmt.Println(ui.Tool(fmt.Sprintf("Executing %d tools...", len(calls))))

	// create a context with timeout for the entire tool execution
	ctx, cancel := context.WithTimeout(t.ctx, 5*time.Minute)
	defer cancel()

	for i, call := range calls {
		select {
		case <-ctx.Done():
			return false, fmt.Errorf("tool execution timed out or was cancelled")
		default:
		}

		fmt.Printf("%s\n", ui.Blue(fmt.Sprintf("📋 Tool %d/%d: %s", i+1, len(calls), call.Name)))

		// execute the tool with individual timeout
		toolCtx, toolCancel := context.WithTimeout(ctx, 30*time.Second)
		
		// channel to receive result with timeout
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
		
		var res string
		var err error
		
		select {
		case result := <-resultChan:
			res = result.res
			err = result.err
		case <-toolCtx.Done():
			err = fmt.Errorf("tool %s timed out after 2 minutes", call.Name)
		}
		
		toolCancel()

		if err != nil {
			fmt.Println(ui.Error(fmt.Sprintf("Error running %s: %v", call.Name, err)))
			if res != "" && !isSilentTool(call.Name) {
				fmt.Printf("%s\n", ui.Dim("Result: "+res))
			}
			t.mu.Lock()
			t.promptData.AddMessage("user", fmt.Sprintf("Error executing %s: %v. Result: %s", call.Name, err, res))
			t.mu.Unlock()
			continue
		}

		// success path
		silent := isSilentTool(call.Name)
		if silent {
			summary := silentToolSummary(call.Name, call.Args, res)
			fmt.Println(ui.Success("Done "+call.Name) + summary)
			// keep output concise for the user but give the full data back to the model
			t.mu.Lock()
			t.promptData.AddMessage("user", fmt.Sprintf("Result of %s: %s", call.Name, res))
			t.mu.Unlock()
		} else {
			fmt.Println(ui.Success("Done " + call.Name))
			if res != "" {
				if call.Name == "attempt_completion" {
					// highlight the final summary for clarity
					fmt.Println(ui.Info(res))
					return true, nil
				} else {
					fmt.Printf("%s\n", ui.Dim("Result: "+res))

					// for non-silent tools, send the full result
					t.mu.Lock()
					t.promptData.AddMessage("user", fmt.Sprintf("Result of %s: %s", call.Name, res))
					t.mu.Unlock()
				}
			}
		}

		if call.Name == "attempt_completion" {
			return true, nil
		}
	}

	return false, nil
}


// ProcessTask is the main loop that queries the AI and executes the requested tools.
func (t *Task) ProcessTask() error {
	defer t.Close()

	for iter := 0; iter < maxIterations; iter++ {
		select {
		case <-t.ctx.Done():
			return fmt.Errorf("task cancelled: %v", t.ctx.Err())
		default:
		}

		// rate limiting for AI API calls
		if err := t.waitForAPIRateLimit(); err != nil {
			return fmt.Errorf("rate limit error: %v", err)
		}

		aiResp, err := t.callAI()
		if err != nil {
			fmt.Println(ui.Error("AI error: " + err.Error()))
			fmt.Println(ui.Info("Please try again or rephrase the request"))
			return err
		}
		log.Printf("AI response: %s", aiResp.Content)

		t.appendAssistantMessage(aiResp.Content)

		toolCalls := aiResp.ToolCalls

		if len(toolCalls) == 0 {
			t.mu.Lock()
			t.noToolCount++
			noToolCount := t.noToolCount
			t.mu.Unlock()

			if noToolCount >= 3 {
				fmt.Println(ui.Warning("AI failed to start the task (no tool invocations). Aborting."))
				return errors.New("ai did not provide tool invocations after multiple attempts")
			}

			fmt.Println(ui.Warning("AI did not use any tools. Forcing tool usage..."))
			t.forceToolUsage()

			continue
		}

		// reset counter after successful tool usage
		t.mu.Lock()
		t.noToolCount = 0
		t.mu.Unlock()

		done, err := t.executeTools(toolCalls)
		if err != nil {
			fmt.Println(ui.Error("Tool execution error: " + err.Error()))
			continue
		}

		if done {
			return nil
		}
	}

	fmt.Println(ui.Warning(fmt.Sprintf("reached the limit of %d attempts, task stopped. Type 'continue' or clarify the request to resume", maxIterations)))

	return nil
}

// waitForAPIRateLimit ensures we don't exceed API rate limits
func (t *Task) waitForAPIRateLimit() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	elapsed := time.Since(t.lastAPICall)
	if elapsed < t.minAPIInterval {
		waitTime := t.minAPIInterval - elapsed
		select {
		case <-time.After(waitTime):
			t.lastAPICall = time.Now()
			return nil
		case <-t.ctx.Done():
			return t.ctx.Err()
		}
	}

	t.lastAPICall = time.Now()
	return nil
}

// callAI calls the AI with timeout and returns native response
func (t *Task) callAI() (*AIResponse, error) {
	// create a context with timeout for the AI call
	ctx, cancel := context.WithTimeout(t.ctx, aiCallTimeout)
	defer cancel()

	// we need to create a copy of promptData for thread safety
	t.mu.RLock()
	promptCopy := *t.promptData
	promptCopy.Messages = make([]Message, len(t.promptData.Messages))
	copy(promptCopy.Messages, t.promptData.Messages)
	t.mu.RUnlock()

	// call the unified GenerateCode method
	return t.client.GenerateCode(ctx, &promptCopy)
}

