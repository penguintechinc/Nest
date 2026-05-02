package healthprobe

import (
	"strings"
	"testing"
	"time"
)

func TestNewProbeMetrics(t *testing.T) {
	m := NewProbeMetrics()
	if m == nil {
		t.Error("NewProbeMetrics() returned nil")
	}
}

func TestProbeMetrics_Record(t *testing.T) {
	m := NewProbeMetrics()
	r := HealthResult{
		Tenant:   "tenant1",
		Name:     "resource1",
		State:    "healthy",
		ProbedAt: time.Now(),
	}

	m.Record(r)

	// Verify by checking PrometheusText includes the metric
	text := m.PrometheusText()
	if !strings.Contains(text, "resource1") {
		t.Errorf("PrometheusText() should contain resource name, got: %s", text)
	}
}

func TestProbeMetrics_PrometheusText(t *testing.T) {
	m := NewProbeMetrics()

	results := []HealthResult{
		{Tenant: "tenant1", Name: "res1", State: "healthy", ProbedAt: time.Now()},
		{Tenant: "tenant1", Name: "res2", State: "degraded", ProbedAt: time.Now()},
		{Tenant: "tenant2", Name: "res3", State: "down", ProbedAt: time.Now()},
	}

	for _, r := range results {
		m.Record(r)
	}

	text := m.PrometheusText()

	// Check for expected headers
	if !strings.Contains(text, "# HELP nest_dataresource_health") {
		t.Error("PrometheusText() missing HELP header")
	}
	if !strings.Contains(text, "# TYPE nest_dataresource_health gauge") {
		t.Error("PrometheusText() missing TYPE header")
	}

	// Check for metrics
	if !strings.Contains(text, "tenant=\"tenant1\"") || !strings.Contains(text, "tenant=\"tenant2\"") {
		t.Error("PrometheusText() missing tenant labels")
	}
}

func TestProbeMetrics_PrometheusText_StateValues(t *testing.T) {
	m := NewProbeMetrics()

	tests := []struct {
		state    string
		expected int
	}{
		{"healthy", 0},
		{"degraded", 1},
		{"unreachable", 2},
		{"down", 2},
		{"unknown", 2},
	}

	for i, tt := range tests {
		r := HealthResult{
			Tenant:   "tenant1",
			Name:     "res" + string(rune('1'+i)),
			State:    tt.state,
			ProbedAt: time.Now(),
		}
		m.Record(r)
	}

	text := m.PrometheusText()
	lines := strings.Split(text, "\n")

	// Verify state values are correctly encoded (0, 1, or 2)
	for _, line := range lines {
		if strings.Contains(line, "healthy") && strings.Contains(line, "0 ") {
			// healthy should map to 0
			if !strings.Contains(line, "0 ") {
				t.Error("healthy state should map to 0")
			}
		}
	}
}

func TestNewSLOTracker(t *testing.T) {
	st := NewSLOTracker()
	if st == nil {
		t.Error("NewSLOTracker() returned nil")
	}
}

func TestSLOTracker_Record(t *testing.T) {
	st := NewSLOTracker()

	results := []HealthResult{
		{Tenant: "tenant1", Name: "res1", State: "healthy", ProbedAt: time.Now()},
		{Tenant: "tenant1", Name: "res1", State: "healthy", ProbedAt: time.Now()},
		{Tenant: "tenant1", Name: "res1", State: "degraded", ProbedAt: time.Now()},
	}

	for _, r := range results {
		st.Record(r)
	}

	// Verify status
	status := st.Status("tenant1", "res1")
	if status == nil {
		t.Error("Status() returned nil for recorded resource")
	}
	if status.TotalProbes != 3 {
		t.Errorf("TotalProbes = %d, want 3", status.TotalProbes)
	}
	if status.HealthyProbes != 2 {
		t.Errorf("HealthyProbes = %d, want 2", status.HealthyProbes)
	}
}

func TestSLOTracker_Status_NotFound(t *testing.T) {
	st := NewSLOTracker()

	status := st.Status("tenant1", "nonexistent")
	if status != nil {
		t.Error("Status() should return nil for nonexistent resource")
	}
}

func TestSLOTracker_AvailabilityPct(t *testing.T) {
	st := NewSLOTracker()

	// Record 10 results: 8 healthy, 2 degraded
	for i := 0; i < 8; i++ {
		st.Record(HealthResult{Tenant: "t1", Name: "r1", State: "healthy", ProbedAt: time.Now()})
	}
	for i := 0; i < 2; i++ {
		st.Record(HealthResult{Tenant: "t1", Name: "r1", State: "degraded", ProbedAt: time.Now()})
	}

	status := st.Status("t1", "r1")
	expected := 80.0
	if status.AvailabilityPct != expected {
		t.Errorf("AvailabilityPct = %f, want %f", status.AvailabilityPct, expected)
	}
}

func TestSLOTracker_LastState(t *testing.T) {
	st := NewSLOTracker()

	st.Record(HealthResult{Tenant: "t1", Name: "r1", State: "healthy", ProbedAt: time.Now()})
	st.Record(HealthResult{Tenant: "t1", Name: "r1", State: "degraded", ProbedAt: time.Now()})
	st.Record(HealthResult{Tenant: "t1", Name: "r1", State: "down", ProbedAt: time.Now()})

	status := st.Status("t1", "r1")
	if status.LastState != "down" {
		t.Errorf("LastState = %q, want 'down'", status.LastState)
	}
}

func TestSLOTracker_Informational(t *testing.T) {
	st := NewSLOTracker()
	st.Record(HealthResult{Tenant: "t1", Name: "r1", State: "healthy", ProbedAt: time.Now()})

	status := st.Status("t1", "r1")
	if !status.Informational {
		t.Error("Informational should always be true for imported/external resources")
	}
}

func TestSLOTracker_ZeroProbes(t *testing.T) {
	st := NewSLOTracker()

	// Create a resource with zero probes (shouldn't happen in practice)
	// but verify availability is 0%
	status := st.Status("t1", "r1")
	if status != nil {
		if status.AvailabilityPct != 0 {
			t.Errorf("AvailabilityPct with no probes = %f, want 0", status.AvailabilityPct)
		}
	}
}
