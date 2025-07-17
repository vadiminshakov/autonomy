package task

import (
	"fmt"
	"sync"
	"time"
)

// PlanStorage manages execution plans
type PlanStorage struct {
	plans      map[string]*ExecutionPlan
	metadata   map[string]*PlanMetadata
	mu         sync.RWMutex
	maxPlans   int
	defaultTTL time.Duration
}

// PlanMetadata stores additional information about execution plans
type PlanMetadata struct {
	ID          string
	Description string
	CreatedAt   time.Time
	ExpiresAt   time.Time
	IsComplete  bool
}

// NewPlanStorage creates a new plan storage with default settings
func NewPlanStorage() *PlanStorage {
	return &PlanStorage{
		plans:      make(map[string]*ExecutionPlan),
		metadata:   make(map[string]*PlanMetadata),
		maxPlans:   10,
		defaultTTL: 24 * time.Hour,
	}
}

// StorePlan saves a plan for later execution
func (ps *PlanStorage) StorePlan(id string, plan *ExecutionPlan, description string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if len(ps.plans) >= ps.maxPlans {
		ps.cleanupOldPlans()
	}

	ps.plans[id] = plan

	now := time.Now()
	ps.metadata[id] = &PlanMetadata{
		ID:          id,
		Description: description,
		CreatedAt:   now,
		ExpiresAt:   now.Add(ps.defaultTTL),
		IsComplete:  false,
	}
}

// GetPlan retrieves a stored plan
func (ps *PlanStorage) GetPlan(id string) (*ExecutionPlan, *PlanMetadata, error) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	plan, exists := ps.plans[id]
	if !exists {
		return nil, nil, fmt.Errorf("plan with ID %s not found", id)
	}

	metadata := ps.metadata[id]
	
	return plan, metadata, nil
}

// RemovePlan removes a plan from storage
func (ps *PlanStorage) RemovePlan(id string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	delete(ps.plans, id)
	delete(ps.metadata, id)
}

// ListPlans returns a list of all stored plans
func (ps *PlanStorage) ListPlans() []*PlanMetadata {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	plans := make([]*PlanMetadata, 0, len(ps.metadata))
	for _, metadata := range ps.metadata {
		plans = append(plans, metadata)
	}

	return plans
}

// MarkPlanComplete marks a plan as completed
func (ps *PlanStorage) MarkPlanComplete(id string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	metadata, exists := ps.metadata[id]
	if !exists {
		return fmt.Errorf("plan with ID %s not found", id)
	}

	metadata.IsComplete = true

	return nil
}

// SetPlanTTL sets the time-to-live for a plan
func (ps *PlanStorage) SetPlanTTL(id string, ttl time.Duration) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	metadata, exists := ps.metadata[id]
	if !exists {
		return fmt.Errorf("plan with ID %s not found", id)
	}

	metadata.ExpiresAt = time.Now().Add(ttl)

	return nil
}

// cleanupOldPlans removes expired or completed plans
func (ps *PlanStorage) cleanupOldPlans() {
	now := time.Now()

	// first, remove expired plans
	for id, metadata := range ps.metadata {
		if now.After(metadata.ExpiresAt) {
			delete(ps.plans, id)
			delete(ps.metadata, id)
		}
	}

	// if we still have too many plans, remove the oldest completed ones
	if len(ps.plans) >= ps.maxPlans {
		var oldestID string
		var oldestTime time.Time

		// find the oldest completed plan
		for id, metadata := range ps.metadata {
			if metadata.IsComplete && (oldestTime.IsZero() || metadata.CreatedAt.Before(oldestTime)) {
				oldestID = id
				oldestTime = metadata.CreatedAt
			}
		}

		// remove it if found
		if oldestID != "" {
			delete(ps.plans, oldestID)
			delete(ps.metadata, oldestID)
		}
	}
}

// GeneratePlanID generates a unique plan ID
func (ps *PlanStorage) GeneratePlanID(prefix string) string {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// generate a unique ID based on timestamp and count
	timestamp := time.Now().UnixNano()
	count := len(ps.plans) + 1

	id := fmt.Sprintf("%s_%d_%d", prefix, timestamp, count)

	// ensure uniqueness
	for {
		if _, exists := ps.plans[id]; !exists {
			break
		}
		count++
		id = fmt.Sprintf("%s_%d_%d", prefix, timestamp, count)
	}

	return id
}

// Global plan storage instance
var (
	globalPlanStorage *PlanStorage
	planStorageOnce   sync.Once
)

// GetPlanStorage returns the global plan storage instance
func GetPlanStorage() *PlanStorage {
	planStorageOnce.Do(func() {
		globalPlanStorage = NewPlanStorage()
	})

	return globalPlanStorage
}
