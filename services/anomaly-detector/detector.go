package main

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

type MetricSample struct {
	MetricName string    `json:"metricName"`
	Resource   string    `json:"resource"`
	Tenant     string    `json:"tenant"`
	Value      float64   `json:"value"`
	Timestamp  time.Time `json:"timestamp"`
}

type Anomaly struct {
	ID           string    `json:"id"`
	MetricName   string    `json:"metricName"`
	Resource     string    `json:"resource"`
	Tenant       string    `json:"tenant"`
	Value        float64   `json:"value"`
	Expected     float64   `json:"expected"`
	DeviationPct float64   `json:"deviationPct"`
	Severity     string    `json:"severity"`
	Description  string    `json:"description"`
	DetectedAt   time.Time `json:"detectedAt"`
}

type Detector struct {
	mu        sync.RWMutex
	samples   map[string][]MetricSample
	anomalies []*Anomaly
}

func NewDetector() *Detector {
	return &Detector{
		samples:   make(map[string][]MetricSample),
		anomalies: make([]*Anomaly, 0),
	}
}

func (d *Detector) AddSample(s MetricSample) {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := s.MetricName + ":" + s.Resource
	d.samples[key] = append(d.samples[key], s)

	// Cap at 1000 per key
	if len(d.samples[key]) > 1000 {
		d.samples[key] = d.samples[key][1:]
	}

	d.detectAnomalies(s)
}

func (d *Detector) detectAnomalies(s MetricSample) {
	key := s.MetricName + ":" + s.Resource
	samples := d.samples[key]

	if len(samples) < 10 {
		return
	}

	last10 := samples[len(samples)-10:]

	// Compute mean and variance
	var sum, sumSq float64
	for _, sample := range last10 {
		sum += sample.Value
	}
	mean := sum / float64(len(last10))

	for _, sample := range last10 {
		diff := sample.Value - mean
		sumSq += diff * diff
	}
	variance := sumSq / float64(len(last10))
	stddev := math.Sqrt(variance)

	if stddev == 0 {
		return
	}

	zScore := math.Abs(s.Value-mean) / stddev

	var severity string
	if zScore > 3 {
		severity = "critical"
	} else if zScore > 2.5 {
		severity = "high"
	} else if zScore > 2 {
		severity = "medium"
	} else {
		return
	}

	var deviationPct float64
	if mean != 0 {
		deviationPct = math.Abs(s.Value-mean) / mean * 100
	}

	anom := &Anomaly{
		ID:           fmt.Sprintf("anom-%d", time.Now().UnixNano()),
		MetricName:   s.MetricName,
		Resource:     s.Resource,
		Tenant:       s.Tenant,
		Value:        s.Value,
		Expected:     mean,
		DeviationPct: deviationPct,
		Severity:     severity,
		Description:  fmt.Sprintf("Z-score: %.2f, deviation: %.1f%%", zScore, deviationPct),
		DetectedAt:   time.Now(),
	}

	d.anomalies = append(d.anomalies, anom)

	// Cap at 10000
	if len(d.anomalies) > 10000 {
		d.anomalies = d.anomalies[1:]
	}
}

func (d *Detector) GetAnomalies(tenant, minSeverity string, limit int) []*Anomaly {
	d.mu.RLock()
	defer d.mu.RUnlock()

	severityOrder := map[string]int{
		"low":      1,
		"medium":   2,
		"high":     3,
		"critical": 4,
	}

	minSevLevel := severityOrder[minSeverity]
	if minSevLevel == 0 && minSeverity != "" {
		minSevLevel = 1
	}

	var result []*Anomaly
	for _, a := range d.anomalies {
		if (tenant == "" || a.Tenant == tenant) &&
			(minSevLevel == 0 || severityOrder[a.Severity] >= minSevLevel) {
			result = append(result, a)
		}
	}

	// Sort by newest first
	sort.Slice(result, func(i, j int) bool {
		return result[i].DetectedAt.After(result[j].DetectedAt)
	})

	if limit == 0 {
		limit = 50
	}
	if len(result) > limit {
		result = result[:limit]
	}

	return result
}

func (d *Detector) AnomalyStats() map[string]int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	stats := map[string]int{
		"low":      0,
		"medium":   0,
		"high":     0,
		"critical": 0,
	}

	for _, a := range d.anomalies {
		if _, ok := stats[a.Severity]; ok {
			stats[a.Severity]++
		}
	}

	return stats
}

func (d *Detector) AnomalyStatsForTenant(tenant string) map[string]int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	stats := map[string]int{
		"low":      0,
		"medium":   0,
		"high":     0,
		"critical": 0,
	}

	for _, a := range d.anomalies {
		if a.Tenant == tenant {
			if _, ok := stats[a.Severity]; ok {
				stats[a.Severity]++
			}
		}
	}

	return stats
}
