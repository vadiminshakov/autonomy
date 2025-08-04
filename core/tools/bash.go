package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func init() {
	Register("bash", BashCommand)
}

const (
	DefaultTimeout  = 1 * 60 * 1000  // 1 minute in milliseconds
	MaxTimeout      = 10 * 60 * 1000 // 10 minutes in milliseconds
	MaxOutputLength = 30000
	BashNoOutput    = "no output"
)

// BashCommand executes a bash command with enhanced security and timeout controls.
func BashCommand(args map[string]interface{}) (string, error) {
	cmdStr, ok := args["command"].(string)
	if !ok || strings.TrimSpace(cmdStr) == "" {
		return "", fmt.Errorf("parameter 'command' must be a non-empty string")
	}

	// get timeout parameter (optional)
	timeout := DefaultTimeout
	if timeoutVal, ok := args["timeout"]; ok {
		if timeoutFloat, ok := timeoutVal.(float64); ok {
			timeout = int(timeoutFloat)
		}
	}
	
	// validate timeout
	if timeout > MaxTimeout {
		timeout = MaxTimeout
	} else if timeout <= 0 {
		timeout = DefaultTimeout
	}

	// security check: prevent banned commands
	if isBannedCommand(cmdStr) {
		return "", fmt.Errorf("execution of command '%s' is blocked for security reasons", cmdStr)
	}

	fmt.Printf("🔧 running bash command: %s\n", cmdStr)

	// create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Millisecond)
	defer cancel()

	// execute command
	startTime := time.Now()
	cmd := exec.CommandContext(ctx, "bash", "-c", cmdStr)
	output, err := cmd.CombinedOutput()
	endTime := time.Now()

	// track command execution
	state := getTaskState()
	state.RecordCommandExecuted(cmdStr)

	// truncate output if too long
	outputStr := string(output)
	if len(outputStr) > MaxOutputLength {
		outputStr = truncateOutput(outputStr)
	}

	// handle timeout
	if ctx.Err() == context.DeadlineExceeded {
		if outputStr != "" {
			outputStr += "\n"
		}
		outputStr += "Command was aborted before completion"
		return outputStr, fmt.Errorf("command exceeded timeout of %d milliseconds", timeout)
	}

	// handle command execution error
	if err != nil {
		if outputStr != "" {
			outputStr += "\n"
		}
		if exitError, ok := err.(*exec.ExitError); ok {
			outputStr += fmt.Sprintf("Exit code %d", exitError.ExitCode())
		} else {
			outputStr += fmt.Sprintf("Error: %v", err)
		}
		
		// still return output even on error, but with error details
		return outputStr, nil
	}

	// return no output message if empty
	if outputStr == "" {
		outputStr = BashNoOutput
	}

	// add working directory info similar to CRUSH
	workingDir, _ := exec.Command("pwd").Output()
	if len(workingDir) > 0 {
		outputStr += fmt.Sprintf("\n\n<cwd>%s</cwd>", strings.TrimSpace(string(workingDir)))
	}

	// create structured response with metadata
	metadata := &BashCommandMetadata{
		Command:   cmdStr,
		StartTime: startTime.UnixMilli(),
		EndTime:   endTime.UnixMilli(),
		Output:    outputStr,
	}

	return CreateStructuredResponse(outputStr, metadata), nil
}

// isBannedCommand checks if a command contains banned patterns similar to CRUSH
func isBannedCommand(cmd string) bool {
	cmdLower := strings.ToLower(strings.TrimSpace(cmd))

	// network/Download tools
	bannedCommands := []string{
		"alias", "aria2c", "axel", "chrome", "curl", "curlie", "firefox",
		"http-prompt", "httpie", "links", "lynx", "nc", "safari", "scp", 
		"ssh", "telnet", "w3m", "wget", "xh",
		// system administration
		"doas", "su", "sudo",
		// package managers
		"apk", "apt", "apt-cache", "apt-get", "dnf", "dpkg", "emerge",
		"home-manager", "makepkg", "opkg", "pacman", "paru", "pkg",
		"pkg_add", "pkg_delete", "portage", "rpm", "yay", "yum", "zypper",
		// system modification
		"at", "batch", "chkconfig", "crontab", "fdisk", "mkfs", "mount",
		"parted", "service", "systemctl", "umount",
		// network configuration
		"firewall-cmd", "ifconfig", "ip", "iptables", "netstat", "pfctl",
		"route", "ufw",
		// dangerous operations
		"rm -rf /", "rm -rf /*", ":(){ :|:& };:", "mv / /dev/null",
		"dd if=/dev/zero", "shutdown", "reboot", "halt", "poweroff",
		"init 0", "init 6", "chmod -R 777 /", "passwd", "userdel", 
		"useradd", "usermod",
	}

	for _, banned := range bannedCommands {
		if strings.Contains(cmdLower, banned) {
			return true
		}
	}

	// check for argument-based blocks
	argumentBlocks := [][]string{
		{"apk", "add"}, {"apt", "install"}, {"apt-get", "install"},
		{"dnf", "install"}, {"emerge"}, {"pacman", "-S"}, {"pkg", "install"},
		{"yum", "install"}, {"zypper", "install"}, {"brew", "install"},
		{"cargo", "install"}, {"gem", "install"}, {"go", "install"},
		{"npm", "install", "-g"}, {"npm", "install", "--global"},
		{"pip", "install", "--user"}, {"pip3", "install", "--user"},
		{"pnpm", "add", "-g"}, {"pnpm", "add", "--global"},
		{"yarn", "global", "add"},
	}

	for _, block := range argumentBlocks {
		if len(block) == 1 {
			if strings.HasPrefix(cmdLower, block[0]+" ") || cmdLower == block[0] {
				return true
			}
		} else {
			allMatch := true
			cmdParts := strings.Fields(cmdLower)
			if len(cmdParts) >= len(block) {
				for i, part := range block {
					if i >= len(cmdParts) || cmdParts[i] != part {
						allMatch = false
						break
					}
				}
				if allMatch {
					return true
				}
			}
		}
	}

	return false
}

// truncateOutput truncates long output similar to CRUSH
func truncateOutput(content string) string {
	if len(content) <= MaxOutputLength {
		return content
	}

	halfLength := MaxOutputLength / 2
	start := content[:halfLength]
	end := content[len(content)-halfLength:]

	truncatedLinesCount := countLines(content[halfLength : len(content)-halfLength])
	return fmt.Sprintf("%s\n\n... [%d lines truncated] ...\n\n%s", start, truncatedLinesCount, end)
}

// countLines counts the number of lines in a string
func countLines(s string) int {
	if s == "" {
		return 0
	}
	return len(strings.Split(s, "\n"))
}

// BashCommandMetadata contains metadata for bash command execution
type BashCommandMetadata struct {
	Command   string `json:"command"`
	StartTime int64  `json:"start_time"`
	EndTime   int64  `json:"end_time"`
	Output    string `json:"output"`
}