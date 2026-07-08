package main

import (
	"sync"
	"time"
)

type WorkloadMetrics struct {
	ResourceID    string  `json:"resourceId"`
	Tenant        string  `json:"tenant"`
	ReadRPS       float64 `json:"readRps"`
	WriteRPS      float64 `json:"writeRps"`
	AvgLatencyMs  float64 `json:"avgLatencyMs"`
	P99LatencyMs  float64 `json:"p99LatencyMs"`
	DataSizeGB    float64 `json:"dataSizeGb"`
	ScanRatio     float64 `json:"scanRatio"`
	MetricsWindow string  `json:"metricsWindow"`
}

type ClassRecommendation struct {
	ResourceID       string    `json:"resourceId"`
	Tenant           string    `json:"tenant"`
	WorkloadType     string    `json:"workloadType"`
	RecommendedClass string    `json:"recommendedClass"`
	Confidence       float64   `json:"confidence"`
	Reason           string    `json:"reason"`
	AutoApply        bool      `json:"autoApply"`
	RecommendedAt    time.Time `json:"recommendedAt"`
}

type Classifier struct {
	mu              sync.RWMutex
	recommendations map[string]*ClassRecommendation
}

func NewClassifier() *Classifier {
	return &Classifier{
		recommendations: make(map[string]*ClassRecommendation),
	}
}

func (c *Classifier) Classify(m WorkloadMetrics) *ClassRecommendation {
	var rec *ClassRecommendation

	if m.ScanRatio > 0.5 && m.WriteRPS < 100 {
		rec = &ClassRecommendation{
			ResourceID:       m.ResourceID,
			Tenant:           m.Tenant,
			WorkloadType:     "olap",
			RecommendedClass: "warehouse-large",
			Confidence:       82.0,
			Reason:           "High scan ratio with low write rate",
			AutoApply:        false,
			RecommendedAt:    time.Now(),
		}
	} else if m.P99LatencyMs < 5 && m.WriteRPS > 1000 {
		rec = &ClassRecommendation{
			ResourceID:       m.ResourceID,
			Tenant:           m.Tenant,
			WorkloadType:     "oltp",
			RecommendedClass: "postgres-ha-3",
			Confidence:       88.0,
			Reason:           "Low latency with high write throughput",
			AutoApply:        true,
			RecommendedAt:    time.Now(),
		}
	} else if m.ReadRPS > 10000 && m.AvgLatencyMs < 1 {
		rec = &ClassRecommendation{
			ResourceID:       m.ResourceID,
			Tenant:           m.Tenant,
			WorkloadType:     "cache",
			RecommendedClass: "valkey-ha",
			Confidence:       85.0,
			Reason:           "Very high read throughput with minimal latency",
			AutoApply:        true,
			RecommendedAt:    time.Now(),
		}
	} else if m.DataSizeGB < 10 && m.WriteRPS > 100 && m.ReadRPS > 100 {
		rec = &ClassRecommendation{
			ResourceID:       m.ResourceID,
			Tenant:           m.Tenant,
			WorkloadType:     "timeseries",
			RecommendedClass: "timeseries-standard",
			Confidence:       78.0,
			Reason:           "Balanced read/write on small dataset",
			AutoApply:        false,
			RecommendedAt:    time.Now(),
		}
	} else {
		rec = &ClassRecommendation{
			ResourceID:       m.ResourceID,
			Tenant:           m.Tenant,
			WorkloadType:     "mixed",
			RecommendedClass: "postgres-standard",
			Confidence:       60.0,
			Reason:           "Mixed or unclear workload pattern",
			AutoApply:        false,
			RecommendedAt:    time.Now(),
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.recommendations[m.ResourceID] = rec

	return rec
}

func (c *Classifier) GetRecommendation(resourceID string) (*ClassRecommendation, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	r, ok := c.recommendations[resourceID]
	return r, ok
}

func (c *Classifier) ListRecommendations(tenant string) []*ClassRecommendation {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var result []*ClassRecommendation
	for _, r := range c.recommendations {
		if tenant != "" && r.Tenant == tenant {
			result = append(result, r)
		}
	}
	return result
}
