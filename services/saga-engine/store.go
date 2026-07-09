package main

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

type SagaTemplate struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Steps       []SagaStep `json:"steps"`
	CreatedAt   time.Time  `json:"createdAt"`
}

type SagaStep struct {
	Name    string            `json:"name"`
	Action  string            `json:"action"`
	Params  map[string]string `json:"params,omitempty"`
	Timeout int               `json:"timeoutSec,omitempty"`
	OnError string            `json:"onError,omitempty"`
}

type SagaRun struct {
	ID          string       `json:"id"`
	TemplateID  string       `json:"templateId"`
	Tenant      string       `json:"tenant"`
	Status      string       `json:"status"`
	CurrentStep int          `json:"currentStep"`
	StepCount   int          `json:"stepCount"`
	StartedAt   time.Time    `json:"startedAt"`
	UpdatedAt   time.Time    `json:"updatedAt"`
	Error       string       `json:"error,omitempty"`
	StepResults []StepResult `json:"stepResults,omitempty"`
}

type StepResult struct {
	Step      string    `json:"step"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"startedAt"`
	EndedAt   time.Time `json:"endedAt,omitempty"`
	Output    string    `json:"output,omitempty"`
	Error     string    `json:"error,omitempty"`
}

type SagaStore struct {
	mu        sync.RWMutex
	templates map[string]*SagaTemplate
	runs      map[string]*SagaRun
	logger    *zap.Logger
}

func NewSagaStore(logger *zap.Logger) *SagaStore {
	return &SagaStore{
		templates: make(map[string]*SagaTemplate),
		runs:      make(map[string]*SagaRun),
		logger:    logger,
	}
}

func (s *SagaStore) CreateTemplate(t *SagaTemplate) error {
	if len(t.Steps) == 0 {
		return fmt.Errorf("template must have at least one step")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	t.ID = fmt.Sprintf("tmpl-%d", time.Now().UnixNano())
	t.CreatedAt = time.Now()
	s.templates[t.ID] = t

	return nil
}

func (s *SagaStore) GetTemplate(id string) (*SagaTemplate, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.templates[id]
	return t, ok
}

func (s *SagaStore) ListTemplates() []*SagaTemplate {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*SagaTemplate, 0, len(s.templates))
	for _, t := range s.templates {
		result = append(result, t)
	}
	return result
}

func (s *SagaStore) StartRun(templateID, tenant string) (SagaRun, error) {
	s.mu.Lock()
	tmpl, ok := s.templates[templateID]
	if !ok {
		s.mu.Unlock()
		return SagaRun{}, fmt.Errorf("template not found")
	}

	run := &SagaRun{
		ID:          fmt.Sprintf("run-%d", time.Now().UnixNano()),
		TemplateID:  templateID,
		Tenant:      tenant,
		Status:      "pending",
		CurrentStep: 0,
		StepCount:   len(tmpl.Steps),
		StartedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		StepResults: make([]StepResult, len(tmpl.Steps)),
	}

	for i, step := range tmpl.Steps {
		run.StepResults[i] = StepResult{
			Step:   step.Name,
			Status: "pending",
		}
	}

	s.runs[run.ID] = run
	// Deep copy StepResults slice to avoid sharing underlying array
	runCopy := *run
	runCopy.StepResults = make([]StepResult, len(run.StepResults))
	for i, sr := range run.StepResults {
		runCopy.StepResults[i] = sr
	}
	s.mu.Unlock()

	go advanceRun(s, run)
	return runCopy, nil
}

func (s *SagaStore) GetRun(id string) (SagaRun, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.runs[id]
	if !ok {
		return SagaRun{}, false
	}
	// Deep copy StepResults slice to avoid sharing underlying array
	copy := *r
	copy.StepResults = make([]StepResult, len(r.StepResults))
	for i, sr := range r.StepResults {
		copy.StepResults[i] = sr
	}
	return copy, true
}

func (s *SagaStore) ListRuns(tenant string) []SagaRun {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]SagaRun, 0)
	for _, r := range s.runs {
		if tenant == "" || r.Tenant == tenant {
			// Deep copy StepResults slice to avoid sharing underlying array
			copy := *r
			copy.StepResults = make([]StepResult, len(r.StepResults))
			for i, sr := range r.StepResults {
				copy.StepResults[i] = sr
			}
			result = append(result, copy)
		}
	}
	return result
}

func (s *SagaStore) RetryRun(id string) error {
	s.mu.Lock()
	run, ok := s.runs[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("run not found")
	}

	if run.Status != "failed" {
		s.mu.Unlock()
		return fmt.Errorf("can only retry failed runs")
	}

	run.Status = "running"
	run.UpdatedAt = time.Now()
	s.mu.Unlock()

	go advanceRun(s, run)
	return nil
}

func (s *SagaStore) CancelRun(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	run, ok := s.runs[id]
	if !ok {
		return fmt.Errorf("run not found")
	}

	if run.Status != "pending" && run.Status != "running" && run.Status != "rolling-back" {
		return fmt.Errorf("cannot cancel run with status %s", run.Status)
	}

	run.Status = "failed"
	run.Error = "cancelled by operator"
	run.UpdatedAt = time.Now()

	return nil
}

func advanceRun(s *SagaStore, run *SagaRun) {
	s.mu.Lock()
	run.Status = "running"
	s.mu.Unlock()

	for {
		s.mu.Lock()
		if run.CurrentStep >= run.StepCount {
			run.Status = "succeeded"
			run.UpdatedAt = time.Now()
			s.mu.Unlock()
			break
		}

		stepResult := &run.StepResults[run.CurrentStep]
		stepResult.Status = "running"
		stepResult.StartedAt = time.Now()
		s.mu.Unlock()

		time.Sleep(500 * time.Millisecond)

		s.mu.Lock()
		stepResult.Status = "succeeded"
		stepResult.EndedAt = time.Now()
		stepResult.Output = fmt.Sprintf("step %s completed", stepResult.Step)
		run.CurrentStep++
		run.UpdatedAt = time.Now()
		s.mu.Unlock()
	}
}
