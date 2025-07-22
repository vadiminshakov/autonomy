package task

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/vadiminshakov/autonomy/core/types"
)

// TaskSummary contains execution results summary
type TaskSummary struct {
	TotalSteps     int
	CompletedSteps int
	FailedSteps    int
	FilesModified  []string
	FilesRead      []string
	KeyFindings    []string
	ExecutionTime  time.Duration
	ParallelGroups int
	Errors         []string
}

// TaskSummarizer generates execution summaries
type TaskSummarizer struct {
	mu sync.RWMutex
}

// NewTaskSummarizer creates a new task summarizer
func NewTaskSummarizer() *TaskSummarizer {
	return &TaskSummarizer{}
}

// GenerateSummary creates a summary from execution plan
//
//nolint:gocyclo
func (ts *TaskSummarizer) GenerateSummary(plan *types.ExecutionPlan) *TaskSummary {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	summary := &TaskSummary{
		TotalSteps:     len(plan.Steps),
		FilesModified:  []string{},
		FilesRead:      []string{},
		KeyFindings:    []string{},
		Errors:         []string{},
		ParallelGroups: len(plan.ParallelGroups),
	}

	// calculate statistics and collect information
	var startTime, endTime *time.Time
	filesModifiedMap := make(map[string]bool)
	filesReadMap := make(map[string]bool)

	for _, step := range plan.Steps {
		// track step status
		switch step.Status {
		case types.StepStatusCompleted:
			summary.CompletedSteps++
		case types.StepStatusFailed:
			summary.FailedSteps++
			if step.Error != nil {
				summary.Errors = append(summary.Errors, fmt.Sprintf("%s: %v", step.ToolName, step.Error))
			}
		}

		// track execution time
		if step.StartTime != nil {
			if startTime == nil || step.StartTime.Before(*startTime) {
				startTime = step.StartTime
			}
		}
		if step.EndTime != nil {
			if endTime == nil || step.EndTime.After(*endTime) {
				endTime = step.EndTime
			}
		}

		// track files
		if path := ts.getFilePath(step); path != "" {
			switch step.ToolName {
			case "read_file":
				filesReadMap[path] = true
			case "write_file", "apply_diff":
				filesModifiedMap[path] = true
			}
		}

		// extract key findings
		if step.Status == types.StepStatusCompleted && step.Result != "" {
			findings := ts.extractKeyFindings(step)
			if len(findings) > 0 {
				summary.KeyFindings = append(summary.KeyFindings, findings...)
			}
		}
	}

	// convert maps to slices
	for file := range filesModifiedMap {
		summary.FilesModified = append(summary.FilesModified, file)
	}
	for file := range filesReadMap {
		summary.FilesRead = append(summary.FilesRead, file)
	}

	// calculate total execution time
	if startTime != nil && endTime != nil {
		summary.ExecutionTime = endTime.Sub(*startTime)
	}

	return summary
}

// FormatSummary creates human-readable summary
func (ts *TaskSummarizer) FormatSummary(summary *TaskSummary) string {
	var sb strings.Builder

	sb.WriteString("# Task Execution Summary\n\n")

	sb.WriteString("## Statistics\n\n")
	sb.WriteString(fmt.Sprintf("- **Total Steps**: %d\n", summary.TotalSteps))
	sb.WriteString(fmt.Sprintf("- **Completed Steps**: %d\n", summary.CompletedSteps))
	sb.WriteString(fmt.Sprintf("- **Failed Steps**: %d\n", summary.FailedSteps))
	sb.WriteString(fmt.Sprintf("- **Execution Time**: %s\n", summary.ExecutionTime.Round(time.Millisecond)))
	sb.WriteString(fmt.Sprintf("- **Parallel Groups**: %d\n", summary.ParallelGroups))

	if len(summary.FilesModified) > 0 {
		sb.WriteString("\n## Files Modified\n\n")
		for _, file := range summary.FilesModified {
			sb.WriteString(fmt.Sprintf("- %s\n", file))
		}
	}

	if len(summary.FilesRead) > 0 {
		sb.WriteString("\n## Files Read\n\n")
		for _, file := range summary.FilesRead {
			sb.WriteString(fmt.Sprintf("- %s\n", file))
		}
	}

	if len(summary.KeyFindings) > 0 {
		sb.WriteString("\n## Key Findings\n\n")
		for _, finding := range summary.KeyFindings {
			sb.WriteString(fmt.Sprintf("- %s\n", finding))
		}
	}

	if len(summary.Errors) > 0 {
		sb.WriteString("\n## Errors\n\n")
		for _, err := range summary.Errors {
			sb.WriteString(fmt.Sprintf("- %s\n", err))
		}
	}

	return sb.String()
}

// getFilePath extracts file path from step arguments
func (ts *TaskSummarizer) getFilePath(step *types.ExecutionStep) string {
	if path, ok := step.Args["path"].(string); ok {
		return path
	}
	if file, ok := step.Args["file"].(string); ok {
		return file
	}
	return ""
}

// extractKeyFindings extracts key findings from step results
func (ts *TaskSummarizer) extractKeyFindings(step *types.ExecutionStep) []string {
	var findings []string

	switch step.ToolName {
	case "analyze_code_go":
		findings = ts.extractAnalysisFindings(step.Result)
	case "search_dir", "search_index":
		findings = ts.extractSearchFindings(step.ToolName, step.Result)
	case "go_test", "go_vet":
		findings = ts.extractTestFindings(step.ToolName, step.Result)
	}

	return findings
}

// extractAnalysisFindings extracts findings from code analysis results
func (ts *TaskSummarizer) extractAnalysisFindings(result string) []string {
	var findings []string

	if strings.Contains(result, "high complexity") {
		findings = append(findings, "Code has high complexity areas")
	}

	if strings.Contains(result, "error handling") {
		findings = append(findings, "Potential error handling issues detected")
	}

	if strings.Contains(result, "performance") {
		findings = append(findings, "Performance concerns identified")
	}

	return findings
}

// extractSearchFindings extracts findings from search results
func (ts *TaskSummarizer) extractSearchFindings(toolName, result string) []string {
	var findings []string

	if strings.Contains(result, "no matches found") {
		findings = append(findings, fmt.Sprintf("%s found no matches", toolName))
		return findings
	}

	// extract match count if available
	for _, line := range strings.Split(result, "\n") {
		if strings.Contains(line, "Found") && strings.Contains(line, "matches") {
			findings = append(findings, strings.TrimSpace(line))
			break
		}
	}

	return findings
}

// extractTestFindings extracts findings from test/vet results
func (ts *TaskSummarizer) extractTestFindings(toolName, result string) []string {
	var findings []string

	switch toolName {
	case "go_test":
		if strings.Contains(result, "FAIL") {
			findings = append(findings, "Tests failed")
		} else if strings.Contains(result, "PASS") {
			findings = append(findings, "All tests passed")
		}

		// extract test coverage if available
		for _, line := range strings.Split(result, "\n") {
			if strings.Contains(line, "coverage") {
				findings = append(findings, strings.TrimSpace(line))
				break
			}
		}

	case "go_vet":
		if strings.Contains(result, "no issues") {
			findings = append(findings, "No issues found by go vet")
		} else {
			findings = append(findings, "Issues found by go vet")
		}
	}

	return findings
}
