package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func init() {
	Register("git_status", GitStatus)
	Register("git_log", GitLog)
	Register("git_diff", GitDiff)
	Register("git_branch", GitBranch)
}

func GitStatus(args map[string]interface{}) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git status failed: %v", err)
	}

	result := string(output)
	if strings.TrimSpace(result) == "" {
		return "Working tree clean – no changes", nil
	}

	return fmt.Sprintf("Git status:\n%s", result), nil
}

func GitLog(args map[string]interface{}) (string, error) {
	count := "10"
	if c, ok := args["count"].(string); ok && c != "" {
		count = c
	} else if c, ok := args["count"].(float64); ok {
		count = fmt.Sprintf("%.0f", c)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "log", "--oneline", "-n", count)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git log failed: %v", err)
	}

	result := string(output)
	if strings.TrimSpace(result) == "" {
		return "No commits found", nil
	}

	return fmt.Sprintf("Last %s commits:\n%s", count, result), nil
}

func GitDiff(args map[string]interface{}) (string, error) {
	var cmdArgs []string
	if file, ok := args["file"].(string); ok && file != "" {
		cmdArgs = []string{"diff", file}
	} else {
		cmdArgs = []string{"diff"}
	}

	if cached, ok := args["cached"].(bool); ok && cached {
		cmdArgs = append(cmdArgs, "--cached")
	} else if cached, ok := args["cached"].(string); ok && (cached == "true" || cached == "1") {
		cmdArgs = append(cmdArgs, "--cached")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("git diff failed: %v", err)
	}

	result := string(output)
	if strings.TrimSpace(result) == "" {
		return "No differences to show", nil
	}

	return fmt.Sprintf("Git diff:\n%s", result), nil
}

//nolint:gocyclo
func GitBranch(args map[string]interface{}) (string, error) {
	action, ok := args["action"].(string)
	if !ok || action == "" {
		action = "list"
	}

	switch action {
	case "list":
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "git", "branch", "-a")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("git branch failed: %v", err)
		}
		return fmt.Sprintf("Git branches:\n%s", string(output)), nil

	case "create":
		name, ok := args["name"].(string)
		if !ok || name == "" {
			return "", fmt.Errorf("parameter 'name' is required to create a branch")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "git", "branch", name)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return string(output), fmt.Errorf("failed to create branch: %v", err)
		}
		return fmt.Sprintf("Branch '%s' created", name), nil

	case "checkout":
		name, ok := args["name"].(string)
		if !ok || name == "" {
			return "", fmt.Errorf("parameter 'name' is required to checkout a branch")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "git", "checkout", name)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return string(output), fmt.Errorf("failed to checkout branch: %v", err)
		}
		return fmt.Sprintf("Switched to branch '%s'\n%s", name, string(output)), nil

	case "delete":
		name, ok := args["name"].(string)
		if !ok || name == "" {
			return "", fmt.Errorf("parameter 'name' is required to delete a branch")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "git", "branch", "-d", name)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return string(output), fmt.Errorf("failed to delete branch: %v", err)
		}
		return fmt.Sprintf("Branch '%s' deleted", name), nil

	default:
		return "", fmt.Errorf("unknown action: %s. Allowed: list, create, checkout, delete", action)
	}
}
