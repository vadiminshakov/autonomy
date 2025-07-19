package tools

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

func init() {
	Register("get_task_state", getTaskStateAsJSON)
	Register("get_task_summary", getTaskSummary)
	Register("reset_task_state", resetTaskState)
	Register("check_tool_usage", checkToolUsage)
}

// taskState tracks the state of task execution
type taskState struct {
	mu               sync.RWMutex
	StartTime        time.Time              `json:"start_time"`
	CompletedTools   map[string]int         `json:"completed_tools"`
	CreatedFiles     []string               `json:"created_files"`
	ModifiedFiles    []string               `json:"modified_files"`
	ReadFiles        []string               `json:"read_files"`
	ExecutedCommands []string               `json:"executed_commands"`
	Errors           []string               `json:"errors"`
	LastToolResult   string                 `json:"last_tool_result"`
	Context          map[string]interface{} `json:"context"`
}

var (
	globalTaskState *taskState
	stateOnce       sync.Once
)

// getTaskStateAsJSON returns the current task state as JSON
func getTaskStateAsJSON(args map[string]interface{}) (string, error) {
	state := getTaskState()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal task state: %v", err)
	}

	return string(data), nil
}

// getTaskSummary returns a human-readable summary of the task state
func getTaskSummary(args map[string]interface{}) (string, error) {
	state := getTaskState()
	return state.GetSummary(), nil
}

// resetTaskState resets the task state
func resetTaskState(args map[string]interface{}) (string, error) {
	state := getTaskState()
	state.Reset()
	return "Task state has been reset", nil
}

// checkToolUsage checks if a tool has been used and how many times
func checkToolUsage(args map[string]interface{}) (string, error) {
	toolName, ok := args["tool"].(string)
	if !ok {
		return "", fmt.Errorf("tool parameter is required")
	}

	state := getTaskState()
	state.mu.RLock()
	count := state.CompletedTools[toolName]
	state.mu.RUnlock()

	if count == 0 {
		return fmt.Sprintf("Tool '%s' has not been used yet", toolName), nil
	}
	return fmt.Sprintf("Tool '%s' has been used %d times", toolName, count), nil
}

// getTaskState returns the global task state instance
func getTaskState() *taskState {
	stateOnce.Do(func() {
		globalTaskState = &taskState{
			StartTime:        time.Now(),
			CompletedTools:   make(map[string]int),
			CreatedFiles:     []string{},
			ModifiedFiles:    []string{},
			ReadFiles:        []string{},
			ExecutedCommands: []string{},
			Errors:           []string{},
			Context:          make(map[string]interface{}),
		}
	})

	return globalTaskState
}

// RecordToolUse records that a tool was used
func (ts *taskState) RecordToolUse(toolName string, success bool, result string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ts.CompletedTools[toolName]++
	ts.LastToolResult = result

	if !success {
		ts.Errors = append(ts.Errors, fmt.Sprintf("%s failed: %s", toolName, result))
	}
}

// RecordFileCreated records that a file was created
func (ts *taskState) RecordFileCreated(path string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	// avoid duplicates
	for _, f := range ts.CreatedFiles {
		if f == path {
			return
		}
	}
	ts.CreatedFiles = append(ts.CreatedFiles, path)
}

// RecordFileModified records that a file was modified
func (ts *taskState) RecordFileModified(path string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	// avoid duplicates
	for _, f := range ts.ModifiedFiles {
		if f == path {
			return
		}
	}
	ts.ModifiedFiles = append(ts.ModifiedFiles, path)
}

// RecordFileRead records that a file was read
func (ts *taskState) RecordFileRead(path string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	// avoid duplicates
	for _, f := range ts.ReadFiles {
		if f == path {
			return
		}
	}
	ts.ReadFiles = append(ts.ReadFiles, path)
}

// RecordCommandExecuted records that a command was executed
func (ts *taskState) RecordCommandExecuted(command string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ts.ExecutedCommands = append(ts.ExecutedCommands, command)
}

// SetContext sets a context value
func (ts *taskState) SetContext(key string, value interface{}) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ts.Context[key] = value
}

// GetContext gets a context value
func (ts *taskState) GetContext(key string) (interface{}, bool) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	val, ok := ts.Context[key]
	return val, ok
}

// GetSummary returns a summary of the task state
func (ts *taskState) GetSummary() string {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	duration := time.Since(ts.StartTime).Round(time.Second)

	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("Task State Summary (Duration: %s)\n", duration))
	summary.WriteString("=====================================\n\n")

	// tool usage
	summary.WriteString("Tools Used:\n")
	for tool, count := range ts.CompletedTools {
		summary.WriteString(fmt.Sprintf("  - %s: %d times\n", tool, count))
	}

	// files
	if len(ts.CreatedFiles) > 0 {
		summary.WriteString("\nCreated Files:\n")
		for _, f := range ts.CreatedFiles {
			summary.WriteString(fmt.Sprintf("  - %s\n", f))
		}
	}

	if len(ts.ModifiedFiles) > 0 {
		summary.WriteString("\nModified Files:\n")
		for _, f := range ts.ModifiedFiles {
			summary.WriteString(fmt.Sprintf("  - %s\n", f))
		}
	}

	if len(ts.ReadFiles) > 0 {
		summary.WriteString(fmt.Sprintf("\nRead %d files\n", len(ts.ReadFiles)))
	}

	// commands
	if len(ts.ExecutedCommands) > 0 {
		summary.WriteString("\nExecuted Commands:\n")
		for _, cmd := range ts.ExecutedCommands {
			if len(cmd) > 60 {
				cmd = cmd[:57] + "..."
			}
			summary.WriteString(fmt.Sprintf("  - %s\n", cmd))
		}
	}

	// errors
	if len(ts.Errors) > 0 {
		summary.WriteString("\nErrors Encountered:\n")
		for _, err := range ts.Errors {
			summary.WriteString(fmt.Sprintf("  - %s\n", err))
		}
	}

	return summary.String()
}

// Reset resets the task state
func (ts *taskState) Reset() {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ts.StartTime = time.Now()
	ts.CompletedTools = make(map[string]int)
	ts.CreatedFiles = []string{}
	ts.ModifiedFiles = []string{}
	ts.ReadFiles = []string{}
	ts.ExecutedCommands = []string{}
	ts.Errors = []string{}
	ts.LastToolResult = ""
	ts.Context = make(map[string]interface{})
}
