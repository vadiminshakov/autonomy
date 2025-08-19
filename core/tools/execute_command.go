package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func init() {
	Register("execute_command", ExecuteCommand)
}

// ExecuteCommand executes a shell command from args["command"]
func ExecuteCommand(args map[string]interface{}) (string, error) {
	cmdStr, ok := args["command"].(string)
	if !ok || strings.TrimSpace(cmdStr) == "" {
		return "", fmt.Errorf("parameter 'command' must be a non-empty string. example: {\"command\": \"go run main.go\"}")
	}

	if isDangerousCommand(cmdStr) {
		return "", fmt.Errorf("execution of command '%s' is blocked for security reasons. use safe alternatives or specify the full path", cmdStr)
	}

	fmt.Printf("running command: %s\n", cmdStr)

	// for interactive commands use shortened timeout
	timeout := 60 * time.Second
	if isInteractiveCommand(cmdStr) {
		timeout = 10 * time.Second
		fmt.Printf("⚡ Using shorter timeout for interactive command\n")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", cmdStr)
	out, err := cmd.CombinedOutput()

	state := getTaskState()
	state.RecordCommandExecuted(cmdStr)

	if ctx.Err() == context.DeadlineExceeded {
		msg := "command exceeded %ds timeout. " +
			"use 'interrupt_command' for long-running commands that need analysis"
		return string(out), fmt.Errorf(msg, int(timeout.Seconds()))
	}

	if err != nil {
		// provide more context about the error
		if exitErr, ok := err.(*exec.ExitError); ok {
			return string(out), fmt.Errorf("command failed with exit code %d. output: %s", exitErr.ExitCode(), string(out))
		}
		return string(out), fmt.Errorf("error executing command: %v. output: %s", err, string(out))
	}
	return string(out), nil
}

func isDangerousCommand(cmd string) bool {
	cmd = strings.ToLower(strings.TrimSpace(cmd))

	dangerousCommands := []string{
		"rm -rf /", "rm -rf /*", ":(){ :|:& };:", "mv / /dev/null",
		"dd if=/dev/zero", "mkfs", "fdisk", "parted", "format",
		"shutdown", "reboot", "halt", "poweroff", "init 0", "init 6",
		"sudo rm -rf", "chmod -R 777 /", "chown -R", "passwd",
		"userdel", "useradd", "usermod", "su root", "sudo su",
	}

	for _, dangerous := range dangerousCommands {
		if strings.Contains(cmd, dangerous) {
			return true
		}
	}

	return false
}

// isInteractiveCommand checks if a command is likely to be interactive or long-running
func isInteractiveCommand(cmd string) bool {
	interactivePatterns := []string{
		"go run", "python", "node", "npm start", "java -jar",
		"./", "serve", "server", "listen", "daemon", "watch",
		"tail -f", "less", "more", "vi", "vim", "nano", "emacs",
		"ssh", "telnet", "ftp", "sftp", "mysql", "psql", "redis-cli",
	}

	cmdLower := strings.ToLower(strings.TrimSpace(cmd))
	for _, pattern := range interactivePatterns {
		if strings.Contains(cmdLower, pattern) {
			return true
		}
	}

	return false
}
