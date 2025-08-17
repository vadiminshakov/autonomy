package tools

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

func init() {
	Register("get_task_state", getTaskStateAsJSON)
	Register("reset_task_state", resetTaskState)
	Register("check_tool_usage", checkToolUsage)
}

// TaskState tracks the state of task execution
type TaskState struct {
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
	globalTaskState *TaskState
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

// GetTaskState returns the global task state instance (exported for external packages)
func GetTaskState() *TaskState {
	return getTaskState()
}

// getTaskState returns the global task state instance
func getTaskState() *TaskState {
	stateOnce.Do(func() {
		globalTaskState = &TaskState{
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
func (ts *TaskState) RecordToolUse(toolName string, success bool, result string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ts.CompletedTools[toolName]++
	ts.LastToolResult = result

	if !success {
		ts.Errors = append(ts.Errors, fmt.Sprintf("%s failed: %s", toolName, result))
	}
}

// RecordFileCreated records that a file was created
func (ts *TaskState) RecordFileCreated(path string) {
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
func (ts *TaskState) RecordFileModified(path string) {
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
func (ts *TaskState) RecordFileRead(path string) {
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
func (ts *TaskState) RecordCommandExecuted(command string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ts.ExecutedCommands = append(ts.ExecutedCommands, command)
}

// SetContext sets a context value
func (ts *TaskState) SetContext(key string, value interface{}) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ts.Context[key] = value
}

// GetContext gets a context value
func (ts *TaskState) GetContext(key string) (interface{}, bool) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	val, ok := ts.Context[key]
	return val, ok
}

// Reset resets the task state
func (ts *TaskState) Reset() {
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
