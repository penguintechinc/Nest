package main

import (
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestCreateTemplate(t *testing.T) {
	logger := zap.NewNop()
	store := NewSagaStore(logger)

	tests := []struct {
		name      string
		template  *SagaTemplate
		wantErr   bool
		errSubstr string
	}{
		{
			name: "success with single step",
			template: &SagaTemplate{
				Name:        "test-template",
				Description: "test description",
				Steps: []SagaStep{
					{Name: "step1", Action: "action1"},
				},
			},
			wantErr: false,
		},
		{
			name: "success with multiple steps",
			template: &SagaTemplate{
				Name: "multi-step",
				Steps: []SagaStep{
					{Name: "step1", Action: "action1"},
					{Name: "step2", Action: "action2"},
					{Name: "step3", Action: "action3"},
				},
			},
			wantErr: false,
		},
		{
			name: "error when steps empty",
			template: &SagaTemplate{
				Name:  "no-steps",
				Steps: []SagaStep{},
			},
			wantErr:   true,
			errSubstr: "must have at least one step",
		},
		{
			name: "error when steps nil",
			template: &SagaTemplate{
				Name:  "nil-steps",
				Steps: nil,
			},
			wantErr:   true,
			errSubstr: "must have at least one step",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.CreateTemplate(tt.template)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errSubstr) {
				t.Errorf("CreateTemplate() error %v does not contain %q", err, tt.errSubstr)
			}
			if !tt.wantErr {
				if tt.template.ID == "" {
					t.Errorf("CreateTemplate() ID not set")
				}
				if tt.template.CreatedAt.IsZero() {
					t.Errorf("CreateTemplate() CreatedAt not set")
				}
			}
		})
	}
}

func TestGetTemplate(t *testing.T) {
	logger := zap.NewNop()
	store := NewSagaStore(logger)

	tmpl := &SagaTemplate{
		Name: "test",
		Steps: []SagaStep{
			{Name: "step1", Action: "action1"},
		},
	}
	store.CreateTemplate(tmpl)
	templateID := tmpl.ID

	tests := []struct {
		name   string
		id     string
		found  bool
		checkID string
	}{
		{
			name:   "template found",
			id:     templateID,
			found:  true,
			checkID: templateID,
		},
		{
			name:  "template not found",
			id:    "nonexistent-id",
			found: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := store.GetTemplate(tt.id)
			if ok != tt.found {
				t.Errorf("GetTemplate() ok = %v, want %v", ok, tt.found)
				return
			}
			if tt.found && got.ID != tt.checkID {
				t.Errorf("GetTemplate() ID = %v, want %v", got.ID, tt.checkID)
			}
		})
	}
}

func TestListTemplates(t *testing.T) {
	logger := zap.NewNop()

	t.Run("empty list", func(t *testing.T) {
		store := NewSagaStore(logger)
		templates := store.ListTemplates()
		if len(templates) != 0 {
			t.Errorf("ListTemplates() on empty store returned %d templates, want 0", len(templates))
		}
	})

	t.Run("multiple templates", func(t *testing.T) {
		freshStore := NewSagaStore(logger)
		var createdTemplates []*SagaTemplate
		for i := 0; i < 3; i++ {
			tmpl := &SagaTemplate{
				Name: "test-" + string(rune('0'+i)),
				Steps: []SagaStep{
					{Name: "step1", Action: "action1"},
				},
			}
			freshStore.CreateTemplate(tmpl)
			createdTemplates = append(createdTemplates, tmpl)
			time.Sleep(1 * time.Millisecond) // Ensure unique timestamps
		}

		templates := freshStore.ListTemplates()
		if len(templates) != 3 {
			t.Errorf("ListTemplates() returned %d templates, want 3", len(templates))
		}

		// Verify all created templates are present
		ids := make(map[string]bool)
		for _, tmpl := range templates {
			ids[tmpl.ID] = true
		}
		if len(ids) != 3 {
			t.Errorf("ListTemplates() returned %d unique IDs, want 3", len(ids))
		}

		// Verify each created template is in the list
		for _, created := range createdTemplates {
			if !ids[created.ID] {
				t.Errorf("ListTemplates() missing created template ID %s", created.ID)
			}
		}
	})
}

func TestStartRun(t *testing.T) {
	logger := zap.NewNop()
	store := NewSagaStore(logger)

	tmpl := &SagaTemplate{
		Name: "test-template",
		Steps: []SagaStep{
			{Name: "step1", Action: "action1"},
			{Name: "step2", Action: "action2"},
		},
	}
	store.CreateTemplate(tmpl)
	templateID := tmpl.ID

	tests := []struct {
		name          string
		templateID    string
		tenant        string
		wantErr       bool
		errSubstr     string
		checkStepCount int
	}{
		{
			name:           "success with valid template",
			templateID:     templateID,
			tenant:         "tenant-1",
			wantErr:        false,
			checkStepCount: 2,
		},
		{
			name:       "error template not found",
			templateID: "nonexistent-template",
			tenant:     "tenant-1",
			wantErr:    true,
			errSubstr:  "template not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run, err := store.StartRun(tt.templateID, tt.tenant)
			if (err != nil) != tt.wantErr {
				t.Errorf("StartRun() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("StartRun() error %v does not contain %q", err, tt.errSubstr)
				}
				return
			}

			if run.ID == "" {
				t.Errorf("StartRun() ID not set")
			}
			if run.TemplateID != tt.templateID {
				t.Errorf("StartRun() TemplateID = %v, want %v", run.TemplateID, tt.templateID)
			}
			if run.Tenant != tt.tenant {
				t.Errorf("StartRun() Tenant = %v, want %v", run.Tenant, tt.tenant)
			}
			if run.Status != "pending" {
				t.Errorf("StartRun() Status = %v, want pending", run.Status)
			}
			if run.StepCount != tt.checkStepCount {
				t.Errorf("StartRun() StepCount = %v, want %v", run.StepCount, tt.checkStepCount)
			}
			if len(run.StepResults) != tt.checkStepCount {
				t.Errorf("StartRun() StepResults length = %v, want %v", len(run.StepResults), tt.checkStepCount)
			}
			if run.StartedAt.IsZero() {
				t.Errorf("StartRun() StartedAt not set")
			}
		})
	}
}

func TestGetRun(t *testing.T) {
	logger := zap.NewNop()
	store := NewSagaStore(logger)

	tmpl := &SagaTemplate{
		Name: "test",
		Steps: []SagaStep{
			{Name: "step1", Action: "action1"},
		},
	}
	store.CreateTemplate(tmpl)

	run, _ := store.StartRun(tmpl.ID, "tenant-1")
	runID := run.ID

	tests := []struct {
		name  string
		id    string
		found bool
	}{
		{
			name:  "run found",
			id:    runID,
			found: true,
		},
		{
			name:  "run not found",
			id:    "nonexistent-run",
			found: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := store.GetRun(tt.id)
			if ok != tt.found {
				t.Errorf("GetRun() ok = %v, want %v", ok, tt.found)
				return
			}
			if tt.found && got.ID != tt.id {
				t.Errorf("GetRun() ID = %v, want %v", got.ID, tt.id)
			}
		})
	}
}

func TestListRuns(t *testing.T) {
	logger := zap.NewNop()
	store := NewSagaStore(logger)

	tmpl := &SagaTemplate{
		Name: "test",
		Steps: []SagaStep{
			{Name: "step1", Action: "action1"},
		},
	}
	store.CreateTemplate(tmpl)

	// Create runs for different tenants - use a fresh store for this test
	freshStore := NewSagaStore(logger)
	freshStore.CreateTemplate(tmpl)

	run1, _ := freshStore.StartRun(tmpl.ID, "tenant-1")
	time.Sleep(1 * time.Millisecond)
	run2, _ := freshStore.StartRun(tmpl.ID, "tenant-1")
	time.Sleep(1 * time.Millisecond)
	run3, _ := freshStore.StartRun(tmpl.ID, "tenant-2")

	tests := []struct {
		name      string
		tenant    string
		wantCount int
		checkIDs  map[string]bool
	}{
		{
			name:      "filter by tenant-1",
			tenant:    "tenant-1",
			wantCount: 2,
			checkIDs:  map[string]bool{run1.ID: true, run2.ID: true},
		},
		{
			name:      "filter by tenant-2",
			tenant:    "tenant-2",
			wantCount: 1,
			checkIDs:  map[string]bool{run3.ID: true},
		},
		{
			name:      "empty tenant returns all",
			tenant:    "",
			wantCount: 3,
			checkIDs:  map[string]bool{run1.ID: true, run2.ID: true, run3.ID: true},
		},
		{
			name:      "nonexistent tenant",
			tenant:    "nonexistent",
			wantCount: 0,
			checkIDs:  map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runs := freshStore.ListRuns(tt.tenant)
			if len(runs) != tt.wantCount {
				t.Errorf("ListRuns() returned %d runs, want %d", len(runs), tt.wantCount)
				return
			}

			for _, run := range runs {
				if !tt.checkIDs[run.ID] {
					t.Errorf("ListRuns() returned unexpected run ID %v", run.ID)
				}
				delete(tt.checkIDs, run.ID)
			}

			if len(tt.checkIDs) != 0 {
				t.Errorf("ListRuns() did not return expected run IDs: %v", tt.checkIDs)
			}
		})
	}
}

func TestRetryRun(t *testing.T) {
	logger := zap.NewNop()

	tmpl := &SagaTemplate{
		Name: "test",
		Steps: []SagaStep{
			{Name: "step1", Action: "action1"},
		},
	}

	tests := []struct {
		name         string
		setupRunFn   func(*SagaStore, string)
		wantErr      bool
		errSubstr    string
		checkStatus  string
	}{
		{
			name: "success retry failed run",
			setupRunFn: func(s *SagaStore, rid string) {
				s.mu.Lock()
				s.runs[rid].Status = "failed"
				s.mu.Unlock()
			},
			wantErr:     false,
			checkStatus: "running",
		},
		{
			name: "error retry non-failed run",
			setupRunFn: func(s *SagaStore, rid string) {
				s.mu.Lock()
				s.runs[rid].Status = "succeeded"
				s.mu.Unlock()
			},
			wantErr:   true,
			errSubstr: "can only retry failed runs",
		},
		{
			name: "error retry not found",
			setupRunFn: func(s *SagaStore, rid string) {
				// nothing
			},
			wantErr:   true,
			errSubstr: "run not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewSagaStore(logger)
			store.CreateTemplate(tmpl)

			if tt.name == "error retry not found" {
				err := store.RetryRun("nonexistent-run-id")
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("RetryRun() error %v does not contain %q", err, tt.errSubstr)
				}
				return
			}

			run, _ := store.StartRun(tmpl.ID, "tenant-1")
			runID := run.ID

			tt.setupRunFn(store, runID)
			err := store.RetryRun(runID)
			if (err != nil) != tt.wantErr {
				t.Errorf("RetryRun() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errSubstr) {
				t.Errorf("RetryRun() error %v does not contain %q", err, tt.errSubstr)
				return
			}
			if !tt.wantErr {
				run, _ := store.GetRun(runID)
				if run.Status != tt.checkStatus {
					t.Errorf("RetryRun() status = %v, want %v", run.Status, tt.checkStatus)
				}
			}
		})
	}
}

func TestCancelRun(t *testing.T) {
	logger := zap.NewNop()
	store := NewSagaStore(logger)

	tmpl := &SagaTemplate{
		Name: "test",
		Steps: []SagaStep{
			{Name: "step1", Action: "action1"},
		},
	}
	store.CreateTemplate(tmpl)

	tests := []struct {
		name        string
		createRun   bool
		runID       string
		setupStatus string
		wantErr     bool
		errSubstr   string
	}{
		{
			name:        "success cancel pending run",
			createRun:   true,
			setupStatus: "pending",
			wantErr:     false,
		},
		{
			name:        "success cancel running run",
			createRun:   true,
			setupStatus: "running",
			wantErr:     false,
		},
		{
			name:        "success cancel rolling-back run",
			createRun:   true,
			setupStatus: "rolling-back",
			wantErr:     false,
		},
		{
			name:        "error cancel succeeded run",
			createRun:   true,
			setupStatus: "succeeded",
			wantErr:     true,
			errSubstr:   "cannot cancel run",
		},
		{
			name:        "error cancel failed run",
			createRun:   true,
			setupStatus: "failed",
			wantErr:     true,
			errSubstr:   "cannot cancel run",
		},
		{
			name:      "error run not found",
			createRun: false,
			runID:     "nonexistent",
			wantErr:   true,
			errSubstr: "run not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runID := tt.runID
			if tt.createRun {
				run, _ := store.StartRun(tmpl.ID, "tenant-1")
				runID = run.ID
				store.mu.Lock()
				store.runs[runID].Status = tt.setupStatus
				store.mu.Unlock()
			}

			err := store.CancelRun(runID)
			if (err != nil) != tt.wantErr {
				t.Errorf("CancelRun() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errSubstr) {
				t.Errorf("CancelRun() error %v does not contain %q", err, tt.errSubstr)
				return
			}
			if !tt.wantErr {
				run, _ := store.GetRun(runID)
				if run.Status != "failed" {
					t.Errorf("CancelRun() status = %v, want failed", run.Status)
				}
				if run.Error != "cancelled by operator" {
					t.Errorf("CancelRun() error message = %v, want cancelled by operator", run.Error)
				}
			}
		})
	}
}

func TestAdvanceRunIntegration(t *testing.T) {
	logger := zap.NewNop()
	store := NewSagaStore(logger)

	tmpl := &SagaTemplate{
		Name: "test-advance",
		Steps: []SagaStep{
			{Name: "step1", Action: "action1"},
		},
	}
	store.CreateTemplate(tmpl)

	run, err := store.StartRun(tmpl.ID, "tenant-1")
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	runID := run.ID

	// Wait for the advance to complete (500ms per step + buffer)
	time.Sleep(1500 * time.Millisecond)

	// Verify run status is succeeded
	finalRun, ok := store.GetRun(runID)
	if !ok {
		t.Fatalf("GetRun() returned not found after advance")
	}

	if finalRun.Status != "succeeded" {
		t.Errorf("advanceRun() final status = %v, want succeeded", finalRun.Status)
	}

	if finalRun.CurrentStep != 1 {
		t.Errorf("advanceRun() CurrentStep = %v, want 1", finalRun.CurrentStep)
	}

	if len(finalRun.StepResults) != 1 {
		t.Errorf("advanceRun() StepResults length = %v, want 1", len(finalRun.StepResults))
		return
	}

	stepResult := finalRun.StepResults[0]
	if stepResult.Status != "succeeded" {
		t.Errorf("advanceRun() step status = %v, want succeeded", stepResult.Status)
	}

	if stepResult.StartedAt.IsZero() {
		t.Errorf("advanceRun() step StartedAt not set")
	}

	if stepResult.EndedAt.IsZero() {
		t.Errorf("advanceRun() step EndedAt not set")
	}

	if stepResult.Output == "" {
		t.Errorf("advanceRun() step Output not set")
	}
}
