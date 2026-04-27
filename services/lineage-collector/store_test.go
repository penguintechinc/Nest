package main

import (
	"testing"
	"time"
)

func TestAddEvent(t *testing.T) {
	store := NewLineageStore()

	event := &LineageEvent{
		EventTime: time.Now(),
		EventType: "START",
		RunID:     "run-123",
		JobName:   "job-1",
		Tenant:    "tenant-1",
	}

	t.Run("success add event", func(t *testing.T) {
		store.Append(event)

		if event.ID == "" {
			t.Errorf("Append() ID not set")
		}

		if event.ReceivedAt.IsZero() {
			t.Errorf("Append() ReceivedAt not set")
		}
	})

	t.Run("auto-generate ID if empty", func(t *testing.T) {
		event2 := &LineageEvent{
			EventTime: time.Now(),
			EventType: "COMPLETE",
			Tenant:    "tenant-1",
		}

		store.Append(event2)

		if event2.ID == "" {
			t.Errorf("Append() auto-generated ID not set")
		}

		if event2.ReceivedAt.IsZero() {
			t.Errorf("Append() auto-generated ReceivedAt not set")
		}
	})

	t.Run("preserve existing ID", func(t *testing.T) {
		event3 := &LineageEvent{
			ID:        "custom-id-123",
			EventTime: time.Now(),
			EventType: "END",
			Tenant:    "tenant-1",
		}

		store.Append(event3)

		if event3.ID != "custom-id-123" {
			t.Errorf("Append() ID = %v, want custom-id-123", event3.ID)
		}
	})
}

func TestQuery(t *testing.T) {
	// Use fresh store for this test
	queryStore := NewLineageStore()

	// Create events
	events := []*LineageEvent{
		{
			EventTime: time.Now().Add(-10 * time.Second),
			EventType: "START",
			RunID:     "run-1",
			JobName:   "job-1",
			Tenant:    "tenant-1",
		},
		{
			EventTime: time.Now().Add(-8 * time.Second),
			EventType: "COMPLETE",
			RunID:     "run-1",
			JobName:   "job-1",
			Tenant:    "tenant-1",
		},
		{
			EventTime: time.Now().Add(-6 * time.Second),
			EventType: "START",
			RunID:     "run-2",
			JobName:   "job-2",
			Tenant:    "tenant-2",
		},
		{
			EventTime: time.Now().Add(-4 * time.Second),
			EventType: "COMPLETE",
			RunID:     "run-1",
			JobName:   "job-3",
			Tenant:    "tenant-1",
		},
		{
			EventTime: time.Now().Add(-2 * time.Second),
			EventType: "START",
			RunID:     "run-3",
			JobName:   "job-1",
			Tenant:    "tenant-1",
		},
	}

	for _, e := range events {
		queryStore.Append(e)
	}

	tests := []struct {
		name      string
		tenant    string
		jobName   string
		runID     string
		limit     int
		wantCount int
		checkFn   func([]*LineageEvent) bool
	}{
		{
			name:      "filter by tenant-1",
			tenant:    "tenant-1",
			jobName:   "",
			runID:     "",
			limit:     100,
			wantCount: 4,
			checkFn: func(results []*LineageEvent) bool {
				for _, e := range results {
					if e.Tenant != "tenant-1" {
						return false
					}
				}
				return true
			},
		},
		{
			name:      "filter by tenant-2",
			tenant:    "tenant-2",
			jobName:   "",
			runID:     "",
			limit:     100,
			wantCount: 1,
			checkFn: func(results []*LineageEvent) bool {
				return results[0].Tenant == "tenant-2"
			},
		},
		{
			name:      "filter by job name",
			tenant:    "",
			jobName:   "job-1",
			runID:     "",
			limit:     100,
			wantCount: 3,
			checkFn: func(results []*LineageEvent) bool {
				for _, e := range results {
					if e.JobName != "job-1" {
						return false
					}
				}
				return true
			},
		},
		{
			name:      "filter by run ID",
			tenant:    "",
			jobName:   "",
			runID:     "run-1",
			limit:     100,
			wantCount: 3,
			checkFn: func(results []*LineageEvent) bool {
				for _, e := range results {
					if e.RunID != "run-1" {
						return false
					}
				}
				return true
			},
		},
		{
			name:      "filter by tenant and job",
			tenant:    "tenant-1",
			jobName:   "job-1",
			runID:     "",
			limit:     100,
			wantCount: 3,
			checkFn: func(results []*LineageEvent) bool {
				for _, e := range results {
					if e.Tenant != "tenant-1" || e.JobName != "job-1" {
						return false
					}
				}
				return true
			},
		},
		{
			name:      "filter by tenant and run ID",
			tenant:    "tenant-1",
			jobName:   "",
			runID:     "run-1",
			limit:     100,
			wantCount: 3,
			checkFn: func(results []*LineageEvent) bool {
				for _, e := range results {
					if e.Tenant != "tenant-1" || e.RunID != "run-1" {
						return false
					}
				}
				return true
			},
		},
		{
			name:      "limit applied",
			tenant:    "tenant-1",
			jobName:   "",
			runID:     "",
			limit:     2,
			wantCount: 2,
			checkFn:   func(results []*LineageEvent) bool { return true },
		},
		{
			name:      "limit 0 defaults to 100",
			tenant:    "",
			jobName:   "",
			runID:     "",
			limit:     0,
			wantCount: 5,
			checkFn:   func(results []*LineageEvent) bool { return true },
		},
		{
			name:      "newest first ordering",
			tenant:    "",
			jobName:   "",
			runID:     "",
			limit:     100,
			wantCount: 5,
			checkFn: func(results []*LineageEvent) bool {
				if len(results) < 2 {
					return true
				}
				// Results should be in reverse chronological order (newest first)
				for i := 0; i < len(results)-1; i++ {
					if results[i].EventTime.Before(results[i+1].EventTime) {
						return false
					}
				}
				return true
			},
		},
		{
			name:      "no matches",
			tenant:    "nonexistent-tenant",
			jobName:   "",
			runID:     "",
			limit:     100,
			wantCount: 0,
			checkFn:   func(results []*LineageEvent) bool { return true },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := queryStore.Query(tt.tenant, tt.jobName, tt.runID, tt.limit)

			if len(results) != tt.wantCount {
				t.Errorf("Query() returned %d events, want %d", len(results), tt.wantCount)
			}

			if !tt.checkFn(results) {
				t.Errorf("Query() returned events that do not match expected criteria")
			}
		})
	}
}

func TestLineageForDataset(t *testing.T) {
	datasetStore := NewLineageStore()

	dataset1 := LineageDataset{Namespace: "s3", Name: "data-lake"}
	dataset2 := LineageDataset{Namespace: "postgres", Name: "users"}
	dataset3 := LineageDataset{Namespace: "s3", Name: "output"}

	// Create events with inputs/outputs
	event1 := &LineageEvent{
		EventTime: time.Now(),
		EventType: "START",
		JobName:   "etl-job",
		Tenant:    "tenant-1",
		Inputs:    []LineageDataset{dataset1},
		Outputs:   []LineageDataset{dataset2},
	}

	event2 := &LineageEvent{
		EventTime: time.Now(),
		EventType: "COMPLETE",
		JobName:   "transform-job",
		Tenant:    "tenant-1",
		Inputs:    []LineageDataset{dataset2},
		Outputs:   []LineageDataset{dataset3},
	}

	event3 := &LineageEvent{
		EventTime: time.Now(),
		EventType: "START",
		JobName:   "export-job",
		Tenant:    "tenant-1",
		Inputs:    []LineageDataset{dataset3},
		Outputs:   []LineageDataset{},
	}

	datasetStore.Append(event1)
	datasetStore.Append(event2)
	datasetStore.Append(event3)

	tests := []struct {
		name      string
		namespace string
		datasetName string
		wantCount int
		checkJobNames func([]*LineageEvent) bool
	}{
		{
			name:      "dataset as input",
			namespace: "s3",
			datasetName: "data-lake",
			wantCount: 1,
			checkJobNames: func(events []*LineageEvent) bool {
				return len(events) == 1 && events[0].JobName == "etl-job"
			},
		},
		{
			name:      "dataset as output and input",
			namespace: "postgres",
			datasetName: "users",
			wantCount: 2,
			checkJobNames: func(events []*LineageEvent) bool {
				if len(events) != 2 {
					return false
				}
				jobNames := make(map[string]bool)
				for _, e := range events {
					jobNames[e.JobName] = true
				}
				return jobNames["etl-job"] && jobNames["transform-job"]
			},
		},
		{
			name:      "dataset as both input and output",
			namespace: "s3",
			datasetName: "output",
			wantCount: 2,
			checkJobNames: func(events []*LineageEvent) bool {
				if len(events) != 2 {
					return false
				}
				jobNames := make(map[string]bool)
				for _, e := range events {
					jobNames[e.JobName] = true
				}
				return jobNames["transform-job"] && jobNames["export-job"]
			},
		},
		{
			name:      "dataset not found",
			namespace: "mysql",
			datasetName: "nonexistent",
			wantCount: 0,
			checkJobNames: func(events []*LineageEvent) bool {
				return true
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := datasetStore.LineageForDataset(tt.namespace, tt.datasetName)

			if len(results) != tt.wantCount {
				t.Errorf("LineageForDataset() returned %d events, want %d", len(results), tt.wantCount)
			}

			if !tt.checkJobNames(results) {
				t.Errorf("LineageForDataset() returned events that do not match expected job names")
			}
		})
	}
}

func TestConcurrentAddEvent(t *testing.T) {
	store := NewLineageStore()

	// Concurrent appends
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			event := &LineageEvent{
				EventTime: time.Now(),
				EventType: "TEST",
				JobName:   "job",
				Tenant:    "tenant-1",
				RunID:     "run-" + string(rune('0'+idx)),
			}
			store.Append(event)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	results := store.Query("tenant-1", "", "", 0)
	if len(results) != 10 {
		t.Errorf("Concurrent appends resulted in %d events, want 10", len(results))
	}
}

func TestQueryWithMultipleFilters(t *testing.T) {
	store := NewLineageStore()

	// Create complex event set
	events := []*LineageEvent{
		{
			EventTime: time.Now().Add(-10 * time.Second),
			EventType: "START",
			RunID:     "run-1",
			JobName:   "job-A",
			Tenant:    "tenant-X",
		},
		{
			EventTime: time.Now().Add(-8 * time.Second),
			EventType: "COMPLETE",
			RunID:     "run-1",
			JobName:   "job-A",
			Tenant:    "tenant-X",
		},
		{
			EventTime: time.Now().Add(-6 * time.Second),
			EventType: "START",
			RunID:     "run-2",
			JobName:   "job-B",
			Tenant:    "tenant-X",
		},
		{
			EventTime: time.Now().Add(-4 * time.Second),
			EventType: "COMPLETE",
			RunID:     "run-1",
			JobName:   "job-A",
			Tenant:    "tenant-Y",
		},
	}

	for _, e := range events {
		store.Append(e)
	}

	// Test: tenant-X AND job-A
	results := store.Query("tenant-X", "job-A", "", 100)
	if len(results) != 2 {
		t.Errorf("Query(tenant-X, job-A) returned %d events, want 2", len(results))
	}

	// Test: tenant-X AND run-1
	results = store.Query("tenant-X", "", "run-1", 100)
	if len(results) != 2 {
		t.Errorf("Query(tenant-X, run-1) returned %d events, want 2", len(results))
	}

	// Test: tenant-X AND job-A AND run-1
	results = store.Query("tenant-X", "job-A", "run-1", 100)
	if len(results) != 2 {
		t.Errorf("Query(tenant-X, job-A, run-1) returned %d events, want 2", len(results))
	}
}

func TestEmptyStore(t *testing.T) {
	store := NewLineageStore()

	t.Run("query empty store", func(t *testing.T) {
		results := store.Query("tenant", "job", "run", 100)
		if len(results) != 0 {
			t.Errorf("Query() on empty store returned %d events, want 0", len(results))
		}
	})

	t.Run("lineage for dataset empty store", func(t *testing.T) {
		results := store.LineageForDataset("namespace", "dataset")
		if len(results) != 0 {
			t.Errorf("LineageForDataset() on empty store returned %d events, want 0", len(results))
		}
	})
}
