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

// ExecuteCommand executes a shell command from args["command"].
func ExecuteCommand(args map[string]interface{}) (string, error) {
	cmdStr, ok := args["command"].(string)
	if !ok || strings.TrimSpace(cmdStr) == "" {
		return "", fmt.Errorf("parameter 'command' must be a non-empty string")
	}

	// security check: prevent dangerous commands
	if isDangerousCommand(cmdStr) {
		return "", fmt.Errorf("execution of command '%s' is blocked for security reasons", cmdStr)
	}

	fmt.Printf("🔧 running command: %s\n", cmdStr)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", cmdStr)
	out, err := cmd.CombinedOutput()

	// Track command execution
	state := getTaskState()
	state.RecordCommandExecuted(cmdStr)

	if ctx.Err() == context.DeadlineExceeded {
		return string(out), fmt.Errorf("command exceeded timeout of 30 seconds")
	}

	if err != nil {
		return string(out), fmt.Errorf("error executing command: %v", err)
	}
	return string(out), nil
}

// isDangerousCommand checks if a command is potentially dangerous
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
