package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

func init() {
	Register("interrupt_command", InterruptCommand)
}

// InterruptCommand interrupts a long-running command and analyzes its output
func InterruptCommand(args map[string]interface{}) (string, error) {
	cmdStr, ok := args["command"].(string)
	if !ok || strings.TrimSpace(cmdStr) == "" {
		return "", fmt.Errorf("parameter 'command' must be a non-empty string")
	}

	if isDangerousCommand(cmdStr) {
		return "", fmt.Errorf("execution of command '%s' is blocked for security reasons", cmdStr)
	}

	fmt.Printf("running command with interrupt capability: %s\n", cmdStr)

	// Create context with shorter timeout for initial execution
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", cmdStr)

	// Capture output in real-time
	var output strings.Builder
	cmd.Stdout = &output
	cmd.Stderr = &output

	// Start command
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start command: %v", err)
	}

	// Wait for completion or timeout
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		// Command completed normally
		state := getTaskState()
		state.RecordCommandExecuted(cmdStr)
		return output.String(), err

	case <-ctx.Done():
		// Command exceeded timeout, interrupt it
		fmt.Println("command exceeded timeout, interrupting...")

		// Try graceful termination first
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			fmt.Printf("failed to send SIGTERM: %v\n", err)
		}

		// Wait a bit for graceful shutdown
		time.Sleep(2 * time.Second)

		// Force kill if still running
		if cmd.Process != nil && cmd.ProcessState == nil {
			if err := cmd.Process.Kill(); err != nil {
				fmt.Printf("failed to kill process: %v\n", err)
			}
		}

		// Record interrupted command
		state := getTaskState()
		state.RecordCommandExecuted(cmdStr + " [INTERRUPTED]")
		state.SetContext("last_interrupted_command", cmdStr)
		state.SetContext("last_command_output", output.String())

		// Analyze the output to understand what happened
		analysis := analyzeCommandOutput(output.String(), cmdStr)

		return fmt.Sprintf("command was interrupted after 10 seconds. output analysis:\n\n%s\n\npartial output:\n%s", analysis, output.String()), nil
	}
}

// analyzeCommandOutput analyzes command output to understand execution progress
func analyzeCommandOutput(output, command string) string {
	if output == "" {
		return "no output captured before interruption"
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		return "no output captured before interruption"
	}

	analysis := []string{}

	// Check for common patterns
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

	// Analyze last few lines for current status
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

	// Estimate progress if possible
	if strings.Contains(command, "go run") || strings.Contains(command, "go build") {
		analysis = append(analysis, "• this appears to be a Go command that may need more time to complete")
	}

	if strings.Contains(command, "npm") || strings.Contains(command, "yarn") {
		analysis = append(analysis, "• this appears to be a Node.js command that may need more time to complete")
	}

	return strings.Join(analysis, "\n")
}
