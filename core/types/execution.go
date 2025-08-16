package types

import (
	"sync"
	"time"
)

type StepStatus string

const (
	StepStatusPending   StepStatus = "pending"
	StepStatusReady     StepStatus = "ready"
	StepStatusRunning   StepStatus = "running"
	StepStatusCompleted StepStatus = "completed"
	StepStatusFailed    StepStatus = "failed"
)

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

	FilesAffected []string `json:"files_affected,omitempty"`
	KeyFindings   []string `json:"key_findings,omitempty"`
	Category      string   `json:"category,omitempty"`

	RiskLevel        string        `json:"risk_level,omitempty"`
	Timeout          time.Duration `json:"timeout,omitempty"`
	PreValidation    bool          `json:"pre_validation,omitempty"`
	PostValidation   bool          `json:"post_validation,omitempty"`
	EnableMonitoring bool          `json:"enable_monitoring,omitempty"`
	CanParallelize   bool          `json:"can_parallelize,omitempty"`
}

type ExecutionPlan struct {
	Steps          []*ExecutionStep `json:"steps"`
	ParallelGroups [][]string       `json:"parallel_groups"`
	Mu             sync.RWMutex     `json:"-"`

	StrategyID    string `json:"strategy_id,omitempty"`
	ExecutionMode string `json:"execution_mode,omitempty"`
}

func (p *ExecutionPlan) RLock() {
	p.Mu.RLock()
}

func (p *ExecutionPlan) RUnlock() {
	p.Mu.RUnlock()
}

func (p *ExecutionPlan) GetSteps() []*ExecutionStep {
	return p.Steps
}
