package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func init() {
	Register("go_test", GoTest)
	Register("go_vet", GoVet)
}

// GoTest runs `go test` for the provided path (defaults to ./...)
func GoTest(args map[string]interface{}) (string, error) {
	path, _ := args["path"].(string)
	if path == "" {
		path = "./..."
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "test", "-v", path)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return out.String(), fmt.Errorf("go test timed out after 5 minutes")
		}
		return out.String(), fmt.Errorf("go test failed: %v", err)
	}

	// summarize result: count of PASS/FAIL
	summaryLines := []string{}
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.HasPrefix(line, "ok ") || strings.HasPrefix(line, "FAIL") {
			summaryLines = append(summaryLines, line)
		}
	}

	return strings.Join(summaryLines, "\n"), nil
}

// GoVet runs `go vet` for the provided path (defaults to ./...)
func GoVet(args map[string]interface{}) (string, error) {
	path, _ := args["path"].(string)
	if path == "" {
		path = "./..."
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "vet", path)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return out.String(), fmt.Errorf("go vet timed out after 2 minutes")
		}
		return out.String(), fmt.Errorf("go vet issues: %v", err)
	}

	if out.Len() == 0 {
		return "go vet: no issues", nil
	}
	return out.String(), nil
}
