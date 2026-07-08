package main

import (
	"testing"
)

func TestNewClassifier(t *testing.T) {
	c := NewClassifier()
	if c == nil {
		t.Fatal("NewClassifier returned nil")
	}
	if c.recommendations == nil {
		t.Fatal("recommendations map is nil")
	}
	if len(c.recommendations) != 0 {
		t.Errorf("expected empty recommendations, got %d", len(c.recommendations))
	}
}

func TestClassifyOLAP(t *testing.T) {
	c := NewClassifier()
	m := WorkloadMetrics{
		ResourceID:    "res1",
		Tenant:        "tenant1",
		ReadRPS:       100.0,
		WriteRPS:      50.0,
		AvgLatencyMs:  10.0,
		P99LatencyMs:  50.0,
		DataSizeGB:    1000.0,
		ScanRatio:     0.8,
		MetricsWindow: "1h",
	}

	rec := c.Classify(m)

	if rec.WorkloadType != "olap" {
		t.Errorf("expected workloadType olap, got %s", rec.WorkloadType)
	}
	if rec.RecommendedClass != "warehouse-large" {
		t.Errorf("expected class warehouse-large, got %s", rec.RecommendedClass)
	}
	if rec.Confidence != 82.0 {
		t.Errorf("expected confidence 82.0, got %f", rec.Confidence)
	}
	if rec.AutoApply != false {
		t.Errorf("expected AutoApply=false for OLAP, got %v", rec.AutoApply)
	}
	if rec.ResourceID != "res1" {
		t.Errorf("expected resourceId res1, got %s", rec.ResourceID)
	}
}

func TestClassifyOLTP(t *testing.T) {
	c := NewClassifier()
	m := WorkloadMetrics{
		ResourceID:    "res2",
		Tenant:        "tenant2",
		ReadRPS:       5000.0,
		WriteRPS:      5000.0,
		AvgLatencyMs:  2.0,
		P99LatencyMs:  2.0,
		DataSizeGB:    100.0,
		ScanRatio:     0.1,
		MetricsWindow: "1h",
	}

	rec := c.Classify(m)

	if rec.WorkloadType != "oltp" {
		t.Errorf("expected workloadType oltp, got %s", rec.WorkloadType)
	}
	if rec.RecommendedClass != "postgres-ha-3" {
		t.Errorf("expected class postgres-ha-3, got %s", rec.RecommendedClass)
	}
	if rec.Confidence != 88.0 {
		t.Errorf("expected confidence 88.0, got %f", rec.Confidence)
	}
	if rec.AutoApply != true {
		t.Errorf("expected AutoApply=true for OLTP, got %v", rec.AutoApply)
	}
}

func TestClassifyCache(t *testing.T) {
	c := NewClassifier()
	m := WorkloadMetrics{
		ResourceID:    "res3",
		Tenant:        "tenant3",
		ReadRPS:       50000.0,
		WriteRPS:      100.0,
		AvgLatencyMs:  0.5,
		P99LatencyMs:  1.0,
		DataSizeGB:    1.0,
		ScanRatio:     0.01,
		MetricsWindow: "1h",
	}

	rec := c.Classify(m)

	if rec.WorkloadType != "cache" {
		t.Errorf("expected workloadType cache, got %s", rec.WorkloadType)
	}
	if rec.RecommendedClass != "valkey-ha" {
		t.Errorf("expected class valkey-ha, got %s", rec.RecommendedClass)
	}
	if rec.Confidence != 85.0 {
		t.Errorf("expected confidence 85.0, got %f", rec.Confidence)
	}
	if rec.AutoApply != true {
		t.Errorf("expected AutoApply=true for cache, got %v", rec.AutoApply)
	}
}

func TestClassifyTimeseries(t *testing.T) {
	c := NewClassifier()
	m := WorkloadMetrics{
		ResourceID:    "res4",
		Tenant:        "tenant4",
		ReadRPS:       200.0,
		WriteRPS:      500.0,
		AvgLatencyMs:  5.0,
		P99LatencyMs:  10.0,
		DataSizeGB:    5.0,
		ScanRatio:     0.2,
		MetricsWindow: "1h",
	}

	rec := c.Classify(m)

	if rec.WorkloadType != "timeseries" {
		t.Errorf("expected workloadType timeseries, got %s", rec.WorkloadType)
	}
	if rec.RecommendedClass != "timeseries-standard" {
		t.Errorf("expected class timeseries-standard, got %s", rec.RecommendedClass)
	}
	if rec.Confidence != 78.0 {
		t.Errorf("expected confidence 78.0, got %f", rec.Confidence)
	}
	if rec.AutoApply != false {
		t.Errorf("expected AutoApply=false for timeseries, got %v", rec.AutoApply)
	}
}

func TestClassifyMixed(t *testing.T) {
	c := NewClassifier()
	m := WorkloadMetrics{
		ResourceID:    "res5",
		Tenant:        "tenant5",
		ReadRPS:       100.0,
		WriteRPS:      100.0,
		AvgLatencyMs:  5.0,
		P99LatencyMs:  10.0,
		DataSizeGB:    500.0,
		ScanRatio:     0.3,
		MetricsWindow: "1h",
	}

	rec := c.Classify(m)

	if rec.WorkloadType != "mixed" {
		t.Errorf("expected workloadType mixed, got %s", rec.WorkloadType)
	}
	if rec.RecommendedClass != "postgres-standard" {
		t.Errorf("expected class postgres-standard, got %s", rec.RecommendedClass)
	}
	if rec.Confidence != 60.0 {
		t.Errorf("expected confidence 60.0, got %f", rec.Confidence)
	}
	if rec.AutoApply != false {
		t.Errorf("expected AutoApply=false for mixed, got %v", rec.AutoApply)
	}
}

func TestClassifyOLAPHighWriteRate(t *testing.T) {
	c := NewClassifier()
	m := WorkloadMetrics{
		ResourceID:    "res6",
		Tenant:        "tenant6",
		ReadRPS:       100.0,
		WriteRPS:      500.0,
		AvgLatencyMs:  10.0,
		P99LatencyMs:  50.0,
		DataSizeGB:    1000.0,
		ScanRatio:     0.8,
		MetricsWindow: "1h",
	}

	rec := c.Classify(m)

	// High write rate (500) contradicts OLAP detection, should fall through
	if rec.WorkloadType == "olap" {
		t.Errorf("expected non-OLAP classification with high write rate, got %s", rec.WorkloadType)
	}
}

func TestGetRecommendation(t *testing.T) {
	c := NewClassifier()
	m := WorkloadMetrics{
		ResourceID:    "res7",
		Tenant:        "tenant7",
		ReadRPS:       100.0,
		WriteRPS:      50.0,
		AvgLatencyMs:  10.0,
		P99LatencyMs:  50.0,
		DataSizeGB:    1000.0,
		ScanRatio:     0.8,
		MetricsWindow: "1h",
	}

	c.Classify(m)

	rec, ok := c.GetRecommendation("res7")
	if !ok {
		t.Fatal("expected recommendation to exist")
	}
	if rec.WorkloadType != "olap" {
		t.Errorf("expected olap, got %s", rec.WorkloadType)
	}
}

func TestGetRecommendationNotFound(t *testing.T) {
	c := NewClassifier()
	_, ok := c.GetRecommendation("nonexistent")
	if ok {
		t.Fatal("expected recommendation not to exist")
	}
}

func TestListRecommendationsEmpty(t *testing.T) {
	c := NewClassifier()
	result := c.ListRecommendations("t1")
	if len(result) != 0 {
		t.Errorf("expected empty list, got %d recommendations", len(result))
	}
}

func TestListRecommendationsAll(t *testing.T) {
	c := NewClassifier()

	// Add 3 recommendations
	for i := 1; i <= 3; i++ {
		m := WorkloadMetrics{
			ResourceID:    "res" + string(rune(i+48)),
			Tenant:        "t1",
			ReadRPS:       100.0,
			WriteRPS:      50.0,
			AvgLatencyMs:  10.0,
			P99LatencyMs:  50.0,
			DataSizeGB:    1000.0,
			ScanRatio:     0.8,
			MetricsWindow: "1h",
		}
		c.Classify(m)
	}

	result := c.ListRecommendations("all")
	if len(result) != 3 {
		t.Errorf("expected 3 recommendations, got %d", len(result))
	}
}

func TestListRecommendationsEmptyString(t *testing.T) {
	c := NewClassifier()

	// Add recommendations
	for i := 1; i <= 2; i++ {
		m := WorkloadMetrics{
			ResourceID:    "res" + string(rune(i+48)),
			Tenant:        "t1",
			ReadRPS:       100.0,
			WriteRPS:      50.0,
			AvgLatencyMs:  10.0,
			P99LatencyMs:  50.0,
			DataSizeGB:    1000.0,
			ScanRatio:     0.8,
			MetricsWindow: "1h",
		}
		c.Classify(m)
	}

	// Empty string should return all
	result := c.ListRecommendations("")
	if len(result) != 2 {
		t.Errorf("expected 2 recommendations with empty filter, got %d", len(result))
	}
}

func TestClassifyBoundaryConditions(t *testing.T) {
	tests := []struct {
		name       string
		metrics    WorkloadMetrics
		expectedWL string
	}{
		{
			name: "scan ratio exactly 0.5",
			metrics: WorkloadMetrics{
				ResourceID:   "res",
				ScanRatio:    0.5,
				WriteRPS:     50.0,
				ReadRPS:      100.0,
				P99LatencyMs: 50.0,
				AvgLatencyMs: 10.0,
				DataSizeGB:   1000.0,
			},
			expectedWL: "mixed",
		},
		{
			name: "scan ratio just above 0.5",
			metrics: WorkloadMetrics{
				ResourceID:   "res",
				ScanRatio:    0.51,
				WriteRPS:     50.0,
				ReadRPS:      100.0,
				P99LatencyMs: 50.0,
				AvgLatencyMs: 10.0,
				DataSizeGB:   1000.0,
			},
			expectedWL: "olap",
		},
		{
			name: "P99 latency exactly 5ms",
			metrics: WorkloadMetrics{
				ResourceID:   "res",
				ScanRatio:    0.1,
				WriteRPS:     1000.0,
				ReadRPS:      1000.0,
				P99LatencyMs: 5.0,
				AvgLatencyMs: 2.0,
				DataSizeGB:   1000.0,
			},
			expectedWL: "mixed",
		},
		{
			name: "P99 latency just below 5ms with high writes",
			metrics: WorkloadMetrics{
				ResourceID:   "res",
				ScanRatio:    0.1,
				WriteRPS:     5000.0,
				ReadRPS:      5000.0,
				P99LatencyMs: 4.9,
				AvgLatencyMs: 2.0,
				DataSizeGB:   100.0,
			},
			expectedWL: "oltp",
		},
		{
			name: "read RPS exactly 10000",
			metrics: WorkloadMetrics{
				ResourceID:   "res",
				ScanRatio:    0.1,
				WriteRPS:     100.0,
				ReadRPS:      10000.0,
				P99LatencyMs: 50.0,
				AvgLatencyMs: 1.0,
				DataSizeGB:   1000.0,
			},
			expectedWL: "mixed",
		},
		{
			name: "read RPS just above 10000",
			metrics: WorkloadMetrics{
				ResourceID:   "res",
				ScanRatio:    0.1,
				WriteRPS:     100.0,
				ReadRPS:      10001.0,
				P99LatencyMs: 50.0,
				AvgLatencyMs: 0.5,
				DataSizeGB:   1000.0,
			},
			expectedWL: "cache",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClassifier()
			rec := c.Classify(tt.metrics)
			if rec.WorkloadType != tt.expectedWL {
				t.Errorf("expected %s, got %s", tt.expectedWL, rec.WorkloadType)
			}
		})
	}
}

func TestClassifyConcurrency(t *testing.T) {
	c := NewClassifier()

	// Classify concurrently
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			m := WorkloadMetrics{
				ResourceID:    "res" + string(rune(idx+48)),
				Tenant:        "t1",
				ReadRPS:       float64(idx*100 + 100),
				WriteRPS:      float64(idx*50 + 50),
				AvgLatencyMs:  5.0,
				P99LatencyMs:  10.0,
				DataSizeGB:    100.0,
				ScanRatio:     0.3,
				MetricsWindow: "1h",
			}
			c.Classify(m)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	if len(c.recommendations) != 10 {
		t.Errorf("expected 10 recommendations, got %d", len(c.recommendations))
	}
}

func TestClassifyUpdatesExistingRecommendation(t *testing.T) {
	c := NewClassifier()
	resourceID := "res1"

	// First classification
	m1 := WorkloadMetrics{
		ResourceID:    resourceID,
		Tenant:        "t1",
		ReadRPS:       100.0,
		WriteRPS:      50.0,
		AvgLatencyMs:  10.0,
		P99LatencyMs:  50.0,
		DataSizeGB:    1000.0,
		ScanRatio:     0.8,
		MetricsWindow: "1h",
	}
	rec1 := c.Classify(m1)

	if rec1.WorkloadType != "olap" {
		t.Fatalf("expected olap, got %s", rec1.WorkloadType)
	}

	// Second classification with different metrics
	m2 := WorkloadMetrics{
		ResourceID:    resourceID,
		Tenant:        "t1",
		ReadRPS:       5000.0,
		WriteRPS:      5000.0,
		AvgLatencyMs:  2.0,
		P99LatencyMs:  2.0,
		DataSizeGB:    100.0,
		ScanRatio:     0.1,
		MetricsWindow: "1h",
	}
	rec2 := c.Classify(m2)

	if rec2.WorkloadType != "oltp" {
		t.Errorf("expected oltp, got %s", rec2.WorkloadType)
	}

	// Verify it was updated
	rec, ok := c.GetRecommendation(resourceID)
	if !ok {
		t.Fatal("expected recommendation to exist")
	}
	if rec.WorkloadType != "oltp" {
		t.Errorf("expected updated recommendation to be oltp, got %s", rec.WorkloadType)
	}
}

func TestRecommendationTimestamp(t *testing.T) {
	c := NewClassifier()
	m := WorkloadMetrics{
		ResourceID:    "res1",
		Tenant:        "t1",
		ReadRPS:       100.0,
		WriteRPS:      50.0,
		AvgLatencyMs:  10.0,
		P99LatencyMs:  50.0,
		DataSizeGB:    1000.0,
		ScanRatio:     0.8,
		MetricsWindow: "1h",
	}

	rec := c.Classify(m)
	if rec.RecommendedAt.IsZero() {
		t.Fatal("expected RecommendedAt timestamp to be set")
	}
}

func TestClassifyAllWorkloadTypes(t *testing.T) {
	testCases := []struct {
		name    string
		metrics WorkloadMetrics
		wlType  string
	}{
		{
			name: "OLAP detection",
			metrics: WorkloadMetrics{
				ResourceID:   "res1",
				ScanRatio:    0.8,
				WriteRPS:     50.0,
				ReadRPS:      100.0,
				P99LatencyMs: 50.0,
				AvgLatencyMs: 10.0,
				DataSizeGB:   1000.0,
			},
			wlType: "olap",
		},
		{
			name: "OLTP detection",
			metrics: WorkloadMetrics{
				ResourceID:   "res2",
				ScanRatio:    0.1,
				WriteRPS:     5000.0,
				ReadRPS:      5000.0,
				P99LatencyMs: 2.0,
				AvgLatencyMs: 1.0,
				DataSizeGB:   100.0,
			},
			wlType: "oltp",
		},
		{
			name: "Cache detection",
			metrics: WorkloadMetrics{
				ResourceID:   "res3",
				ScanRatio:    0.01,
				WriteRPS:     100.0,
				ReadRPS:      50000.0,
				P99LatencyMs: 1.0,
				AvgLatencyMs: 0.5,
				DataSizeGB:   1.0,
			},
			wlType: "cache",
		},
		{
			name: "Timeseries detection",
			metrics: WorkloadMetrics{
				ResourceID:   "res4",
				ScanRatio:    0.2,
				WriteRPS:     500.0,
				ReadRPS:      200.0,
				P99LatencyMs: 10.0,
				AvgLatencyMs: 5.0,
				DataSizeGB:   5.0,
			},
			wlType: "timeseries",
		},
		{
			name: "Mixed detection",
			metrics: WorkloadMetrics{
				ResourceID:   "res5",
				ScanRatio:    0.3,
				WriteRPS:     100.0,
				ReadRPS:      100.0,
				P99LatencyMs: 10.0,
				AvgLatencyMs: 5.0,
				DataSizeGB:   500.0,
			},
			wlType: "mixed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			classifier := NewClassifier()
			rec := classifier.Classify(tc.metrics)
			if rec.WorkloadType != tc.wlType {
				t.Errorf("expected %s, got %s", tc.wlType, rec.WorkloadType)
			}
		})
	}
}
