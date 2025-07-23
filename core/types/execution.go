package types

import (
	"sync"
	"time"
)

// StepStatus represents the status of an execution step
type StepStatus string

const (
	StepStatusPending   StepStatus = "pending"
	StepStatusReady     StepStatus = "ready"
	StepStatusRunning   StepStatus = "running"
	StepStatusCompleted StepStatus = "completed"
	StepStatusFailed    StepStatus = "failed"
)

// ExecutionStep represents a single step in the execution plan
type ExecutionStep struct {
	ID           string         `json:"id"`
	ToolName     string         `json:"tool_name"`
	Args         map[string]any `json:"args"`
	Dependencies []string       `json:"dependencies"`
	Status       StepStatus     `json:"status"`
	Result       string         `json:"result,omitempty"`
	Error        error          `json:"error,omitempty"`
	StartTime    *time.Time     `json:"start_time,omitempty"`
	EndTime      *time.Time     `json:"end_time,omitempty"`

	// Additional fields for reflection
	FilesAffected []string `json:"files_affected,omitempty"`
	KeyFindings   []string `json:"key_findings,omitempty"`
	Category      string   `json:"category,omitempty"` // "analysis", "modification", "test"
}

// ExecutionPlan represents a complete execution plan with dependencies
type ExecutionPlan struct {
	Steps          []*ExecutionStep `json:"steps"`
	ParallelGroups [][]string       `json:"parallel_groups"`
	Mu             sync.RWMutex     `json:"-"`
}

// RLock provides read lock access to the plan
func (p *ExecutionPlan) RLock() {
	p.Mu.RLock()
}

// RUnlock releases the read lock
func (p *ExecutionPlan) RUnlock() {
	p.Mu.RUnlock()
}

// GetSteps returns the execution steps
func (p *ExecutionPlan) GetSteps() []*ExecutionStep {
	return p.Steps
}
