package healthprobe

import (
	"fmt"
	"sync"
	"time"
)

// ProbeMetrics tracks per-resource probe results for Prometheus exposition.
// In production this integrates with the bundled LGTP stack (§20).
type ProbeMetrics struct {
	mu      sync.RWMutex
	results map[string]HealthResult // key: tenant/name
}

// NewProbeMetrics creates a new in-memory metrics store.
func NewProbeMetrics() *ProbeMetrics {
	return &ProbeMetrics{
		results: make(map[string]HealthResult),
	}
}

// Record stores a health result.
func (m *ProbeMetrics) Record(r HealthResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.results[r.Tenant+"/"+r.Name] = r
}

// PrometheusText returns a Prometheus text format exposition of probe metrics.
// Matches the nest_dataresource_health metric described in §20.2.
func (m *ProbeMetrics) PrometheusText() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stateValue := map[string]int{"healthy": 0, "degraded": 1, "unreachable": 2, "down": 2}

	out := "# HELP nest_dataresource_health Health state of DataResources (0=healthy 1=degraded 2=down)\n"
	out += "# TYPE nest_dataresource_health gauge\n"
	for _, r := range m.results {
		val := 2
		if v, ok := stateValue[r.State]; ok {
			val = v
		}
		out += fmt.Sprintf(
			"nest_dataresource_health{tenant=%q,resource=%q,origination=%q} %d %d\n",
			r.Tenant, r.Name, "imported", val, r.ProbedAt.UnixMilli(),
		)
	}
	return out
}

// SLOStatus returns whether a resource is within its informational SLO (§43.3 step 6).
// For imported/external, SLOs are tracked but not enforced.
type SLOStatus struct {
	Tenant          string
	Resource        string
	Informational   bool
	AvailabilityPct float64
	TotalProbes     int
	HealthyProbes   int
	LastState       string
	LastProbedAt    time.Time
}

// SLOTracker computes rolling availability for imported/external resources.
type SLOTracker struct {
	mu      sync.RWMutex
	windows map[string]*sloWindow
}

type sloWindow struct {
	total   int
	healthy int
	last    HealthResult
}

// NewSLOTracker creates a new SLO tracker.
func NewSLOTracker() *SLOTracker {
	return &SLOTracker{
		windows: make(map[string]*sloWindow),
	}
}

// Record records a probe result in the rolling SLO window.
func (t *SLOTracker) Record(r HealthResult) {
	t.mu.Lock()
	defer t.mu.Unlock()
	k := r.Tenant + "/" + r.Name
	w, ok := t.windows[k]
	if !ok {
		w = &sloWindow{}
		t.windows[k] = w
	}
	w.total++
	if r.State == "healthy" {
		w.healthy++
	}
	w.last = r
}

// Status returns the SLO status for a resource.
func (t *SLOTracker) Status(tenant, name string) *SLOStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()
	w, ok := t.windows[tenant+"/"+name]
	if !ok {
		return nil
	}
	var pct float64
	if w.total > 0 {
		pct = float64(w.healthy) / float64(w.total) * 100
	}
	return &SLOStatus{
		Tenant:          tenant,
		Resource:        name,
		Informational:   true,
		AvailabilityPct: pct,
		TotalProbes:     w.total,
		HealthyProbes:   w.healthy,
		LastState:       w.last.State,
		LastProbedAt:    w.last.ProbedAt,
	}
}
