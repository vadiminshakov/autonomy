package tools

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTaskState(t *testing.T) {
	// reset state before test
	state := getTaskState()
	state.Reset()

	// test initial state
	require.Equal(t, 0, len(state.CompletedTools), "Initial state should have no completed tools")

	// test recording tool use
	state.RecordToolUse("test_tool", true, "success")
	require.Equal(t, 1, state.CompletedTools["test_tool"], "Tool use not recorded correctly")

	// test recording file operations
	state.RecordFileCreated("test.txt")
	state.RecordFileModified("main.go")
	state.RecordFileRead("README.md")

	require.Equal(t, 1, len(state.CreatedFiles), "File creation not recorded correctly")
	require.Equal(t, "test.txt", state.CreatedFiles[0], "File creation not recorded correctly")
	require.Equal(t, 1, len(state.ModifiedFiles), "File modification not recorded correctly")
	require.Equal(t, "main.go", state.ModifiedFiles[0], "File modification not recorded correctly")
	require.Equal(t, 1, len(state.ReadFiles), "File read not recorded correctly")
	require.Equal(t, "README.md", state.ReadFiles[0], "File read not recorded correctly")

	// test duplicate prevention
	state.RecordFileCreated("test.txt")
	require.Equal(t, 1, len(state.CreatedFiles), "Duplicate file creation should be prevented")

	// test command recording
	state.RecordCommandExecuted("go test")
	require.Equal(t, 1, len(state.ExecutedCommands), "Command execution not recorded correctly")
	require.Equal(t, "go test", state.ExecutedCommands[0], "Command execution not recorded correctly")

	// test context
	state.SetContext("key", "value")
	val, ok := state.GetContext("key")
	require.True(t, ok, "Context not set/retrieved correctly")
	require.Equal(t, "value", val, "Context not set/retrieved correctly")

	// test error recording
	state.RecordToolUse("failing_tool", false, "error message")
	require.Equal(t, 1, len(state.Errors), "Error not recorded correctly")
}

func TestTaskStateTools(t *testing.T) {
	// reset state
	state := getTaskState()
	state.Reset()

	// test get_task_state tool
	result, err := getTaskStateAsJSON(map[string]interface{}{})
	require.NoError(t, err, "get_task_state failed")

	var stateData map[string]interface{}
	err = json.Unmarshal([]byte(result), &stateData)
	require.NoError(t, err, "Failed to parse task state JSON")

	_, ok := stateData["start_time"]
	require.True(t, ok, "Task state should contain start_time")

	// test check_tool_usage tool
	usage, err := checkToolUsage(map[string]interface{}{"tool": "test_tool"})
	require.NoError(t, err, "check_tool_usage failed")
	require.Contains(t, usage, "has been used 1 times", "Tool usage check incorrect")

	// test checking unused tool
	usage, err = checkToolUsage(map[string]interface{}{"tool": "unused_tool"})
	require.NoError(t, err, "check_tool_usage failed")
	require.Contains(t, usage, "has not been used yet", "Unused tool check incorrect")

	// test missing parameter
	_, err = checkToolUsage(map[string]interface{}{})
	require.Error(t, err, "check_tool_usage should fail without tool parameter")

	// test reset_task_state tool
	_, err = resetTaskState(map[string]interface{}{})
	require.NoError(t, err, "reset_task_state failed")
	require.Equal(t, 0, len(state.CompletedTools), "State should be reset")
}

func TestTaskStateConcurrency(t *testing.T) {
	state := getTaskState()
	state.Reset()

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(n int) {
			state.RecordToolUse("concurrent_tool", true, "success")
			state.RecordFileCreated(string(rune('a'+n)) + ".txt")
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	require.Equal(t, 10, state.CompletedTools["concurrent_tool"], "Expected 10 tool uses")
	require.Equal(t, 10, len(state.CreatedFiles), "Expected 10 created files")
}
