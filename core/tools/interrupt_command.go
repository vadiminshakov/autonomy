package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

type safeWriter struct {
	builder *strings.Builder
	mu      *sync.RWMutex
}

func (sw *safeWriter) Write(p []byte) (n int, err error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.builder.Write(p)
}

func init() {
	Register("interrupt_command", InterruptCommand)
}

func InterruptCommand(args map[string]interface{}) (string, error) {
	cmdStr, ok := args["command"].(string)
	if !ok || strings.TrimSpace(cmdStr) == "" {
		return "", fmt.Errorf("parameter 'command' must be a non-empty string")
	}

	dangerous := []string{"rm -rf", "format", "del /", "shutdown", "reboot", "mkfs"}
	for _, danger := range dangerous {
		if strings.Contains(strings.ToLower(cmdStr), danger) {
			return "", fmt.Errorf("execution of command '%s' is blocked for security reasons", cmdStr)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", cmdStr)

	var output strings.Builder
	var outputMu sync.RWMutex
	
	cmd.Stdout = &safeWriter{builder: &output, mu: &outputMu}
	cmd.Stderr = &safeWriter{builder: &output, mu: &outputMu}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start command: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		state := getTaskState()
		state.RecordCommandExecuted(cmdStr)
		outputMu.RLock()
		result := output.String()
		outputMu.RUnlock()
		return result, err

	case <-ctx.Done():
		if cmd.Process != nil {
			cmd.Process.Signal(syscall.SIGTERM) //nolint:staticcheck
			time.Sleep(2 * time.Second)
			cmd.Process.Kill() //nolint:staticcheck
		}

		outputMu.RLock()
		outputStr := output.String()
		outputMu.RUnlock()

		state := getTaskState()
		state.RecordCommandExecuted(cmdStr + " [INTERRUPTED]")
		state.SetContext("last_interrupted_command", cmdStr)
		state.SetContext("last_command_output", outputStr)

		analysis := analyzeCommandOutput(outputStr, cmdStr)
		return fmt.Sprintf("command was interrupted after 10 seconds. output analysis:\n\n%s\n\npartial output:\n%s", analysis, outputStr), nil
	}
}

func analyzeCommandOutput(output, command string) string {
	if output == "" {
		return "no output captured before interruption"
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		return "no output captured before interruption"
	}

	analysis := []string{}

	if strings.Contains(strings.ToLower(output), "error") {
		analysis = append(analysis, "• errors detected in output")
	}

	if strings.Contains(strings.ToLower(output), "warning") {
		analysis = append(analysis, "• warnings detected in output")
	}

	if strings.Contains(strings.ToLower(output), "success") || strings.Contains(strings.ToLower(output), "completed") {
		analysis = append(analysis, "• command appears to have completed successfully")
	}

	if strings.Contains(strings.ToLower(output), "running") || strings.Contains(strings.ToLower(output), "processing") {
		analysis = append(analysis, "• command was still running when interrupted")
	}

	if strings.Contains(strings.ToLower(output), "building") || strings.Contains(strings.ToLower(output), "compiling") {
		analysis = append(analysis, "• command was in build/compilation phase")
	}

	if strings.Contains(strings.ToLower(output), "downloading") || strings.Contains(strings.ToLower(output), "fetching") {
		analysis = append(analysis, "• command was downloading or fetching resources")
	}

	if strings.Contains(strings.ToLower(output), "testing") || strings.Contains(strings.ToLower(output), "running tests") {
		analysis = append(analysis, "• command was running tests")
	}

	lastLines := lines
	if len(lines) > 5 {
		lastLines = lines[len(lines)-5:]
	}

	analysis = append(analysis, "• last output lines:")
	for _, line := range lastLines {
		if strings.TrimSpace(line) != "" {
			analysis = append(analysis, "  "+strings.TrimSpace(line))
		}
	}

	if strings.Contains(command, "go run") || strings.Contains(command, "go build") {
		analysis = append(analysis, "• this appears to be a Go command that may need more time to complete")
	}

	if strings.Contains(command, "npm") || strings.Contains(command, "yarn") {
		analysis = append(analysis, "• this appears to be a Node.js command that may need more time to complete")
	}

	return strings.Join(analysis, "\n")
}