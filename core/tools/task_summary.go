package tools

import (
	"fmt"
	"strings"
	"time"
)

func init() {
	Register("get_detailed_task_summary", GetDetailedTaskSummary)
}

// TaskSummaryData contains execution results summary
type TaskSummaryData struct {
	TotalSteps     int
	CompletedSteps int
	FailedSteps    int
	FilesModified  []string
	FilesRead      []string
	KeyFindings    []string
	ExecutionTime  time.Duration
	Errors         []string
}

// GetDetailedTaskSummary generates a detailed summary of task execution
func GetDetailedTaskSummary(args map[string]interface{}) (string, error) {
	var sb strings.Builder
	sb.WriteString("# Task Execution Summary\n\n")

	// try to extract summary data from args if available
	if summaryData, exists := args["summary_data"]; exists {
		if data, ok := summaryData.(*TaskSummaryData); ok {
			return formatTaskSummary(data), nil
		}
	}

	// fallback: generate basic summary from available context
	sb.WriteString("## Status\n")
	sb.WriteString("- Task execution completed\n")

	if duration, exists := args["duration"]; exists {
		if d, ok := duration.(time.Duration); ok {
			sb.WriteString(fmt.Sprintf("- Execution time: %s\n", d.Round(time.Millisecond)))
		}
	}

	if steps, exists := args["total_steps"]; exists {
		if s, ok := steps.(int); ok {
			sb.WriteString(fmt.Sprintf("- Total steps executed: %d\n", s))
		}
	}

	if errors, exists := args["errors"]; exists {
		if errs, ok := errors.([]string); ok && len(errs) > 0 {
			sb.WriteString("\n## Errors\n")
			for _, err := range errs {
				sb.WriteString(fmt.Sprintf("- %s\n", err))
			}
		}
	}

	return sb.String(), nil
}

// formatTaskSummary creates human-readable summary from TaskSummaryData
func formatTaskSummary(summary *TaskSummaryData) string {
	var sb strings.Builder

	sb.WriteString("# Task Execution Summary\n\n")

	// statistics
	sb.WriteString("## Statistics\n\n")
	sb.WriteString(fmt.Sprintf("- **Total Steps**: %d\n", summary.TotalSteps))
	sb.WriteString(fmt.Sprintf("- **Completed Steps**: %d\n", summary.CompletedSteps))
	sb.WriteString(fmt.Sprintf("- **Failed Steps**: %d\n", summary.FailedSteps))
	sb.WriteString(fmt.Sprintf("- **Execution Time**: %s\n", summary.ExecutionTime.Round(time.Millisecond)))

	// files
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

	// key Findings
	if len(summary.KeyFindings) > 0 {
		sb.WriteString("\n## Key Findings\n\n")
		for _, finding := range summary.KeyFindings {
			sb.WriteString(fmt.Sprintf("- %s\n", finding))
		}
	}

	// errors
	if len(summary.Errors) > 0 {
		sb.WriteString("\n## Errors\n\n")
		for _, err := range summary.Errors {
			sb.WriteString(fmt.Sprintf("- %s\n", err))
		}
	}

	return sb.String()
}
