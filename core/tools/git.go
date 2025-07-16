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
	Register("git_add", GitAdd)
	Register("git_commit", GitCommit)
	Register("git_log", GitLog)
	Register("git_diff", GitDiff)
	Register("git_branch", GitBranch)
	Register("git_merge_request_review", GitMergeRequestReview)
}

// GitStatus shows the working tree status.
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

// GitAdd stages files.
func GitAdd(args map[string]interface{}) (string, error) {
	files, ok := args["files"].(string)
	if !ok || files == "" {
		files = "." // default: stage all files
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "add", files)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("git add failed: %v", err)
	}

	return fmt.Sprintf("Files staged: %s", files), nil
}

// GitCommit creates a commit.
func GitCommit(args map[string]interface{}) (string, error) {
	message, ok := args["message"].(string)
	if !ok || message == "" {
		return "", fmt.Errorf("parameter 'message' is required for git_commit")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "commit", "-m", message)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("git commit failed: %v", err)
	}

	return fmt.Sprintf("Commit created: %s\n%s", message, string(output)), nil
}

// GitLog shows commit history.
func GitLog(args map[string]interface{}) (string, error) {
	// determine how many commits to show
	count := "10" // default
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

// GitDiff shows changes.
func GitDiff(args map[string]interface{}) (string, error) {
	// show diff for a specific file or the whole repo
	var cmdArgs []string
	if file, ok := args["file"].(string); ok && file != "" {
		cmdArgs = []string{"diff", file}
	} else {
		cmdArgs = []string{"diff"}
	}

	// add --cached option to display index changes
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

// GitBranch manages branches.
func GitBranch(args map[string]interface{}) (string, error) {
	action, ok := args["action"].(string)
	if !ok || action == "" {
		action = "list" // default action
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

// GitMergeRequestReview interacts with GitHub CLI to list or view pull requests.
// Supported actions: list (default) and view (requires 'number').
func GitMergeRequestReview(args map[string]interface{}) (string, error) {
	action, _ := args["action"].(string)
	if action == "" {
		action = "list"
	}

	// ensure we are inside a git repository
	ctx1, cancel1 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel1()
	if err := exec.CommandContext(ctx1, "git", "rev-parse", "--git-dir").Run(); err != nil {
		return "", fmt.Errorf("not a git repository: %v", err)
	}

	switch action {
	case "list":
		// list pull request refs (GitHub style) if they exist
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "git", "ls-remote", "--quiet", "--refs", "origin", "refs/pull/*/head")
		out, err := cmd.CombinedOutput()
		if err != nil {
			return string(out), fmt.Errorf("git ls-remote failed: %v", err)
		}
		if len(out) == 0 {
			return "no PR refs (refs/pull) found in origin", nil
		}

		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		return fmt.Sprintf("pull request refs:\n%s", strings.Join(lines, "\n")), nil

	case "view":
		number, ok := args["number"].(string)
		if !ok || number == "" {
			return "", fmt.Errorf("parameter 'number' is required for view action")
		}

		// fetch the pull request ref into a temporary branch
		prRef := fmt.Sprintf("refs/pull/%s/head", number)
		branch := fmt.Sprintf("pr-%s", number)

		// fetch quietly
		ctx1, cancel1 := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel1()
		if err := exec.CommandContext(ctx1, "git", "fetch", "origin", fmt.Sprintf("%s:%s", prRef, branch)).Run(); err != nil {
			return "", fmt.Errorf("failed to fetch PR: %v", err)
		}

		// show latest commit message for the PR branch
		ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel2()

		cmd := exec.CommandContext(ctx2, "git", "log", "--oneline", "-n", "5", branch)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return string(out), fmt.Errorf("git log failed: %v", err)
		}

		return fmt.Sprintf("last commits in PR #%s:\n%s", number, string(out)), nil

	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}
