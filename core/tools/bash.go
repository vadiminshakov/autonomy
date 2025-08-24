package tools

import (
	"fmt"
	"os/exec"
	"strings"
)

func init() {
	Register("bash", bashCommand)
}

func bashCommand(args map[string]interface{}) (string, error) {
	command, ok := args["command"].(string)
	if !ok {
		return "", fmt.Errorf("command parameter is required")
	}

	cmd := exec.Command("bash", "-c", command)
	output, err := cmd.CombinedOutput()

	result := strings.TrimSpace(string(output))
	if err != nil {
		return result, fmt.Errorf("command failed: %v", err)
	}

	return result, nil
}
