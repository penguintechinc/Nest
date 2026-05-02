package main

import (
	"sync"
	"time"
)

type UtilizationSample struct {
	ResourceID string    `json:"resourceId"`
	Timestamp  time.Time `json:"timestamp"`
	CPUPercent float64   `json:"cpuPercent"`
	MemPercent float64   `json:"memPercent"`
	StoragePct float64   `json:"storagePct"`
}

type ScaleForecast struct {
	ResourceID       string    `json:"resourceId"`
	HoursAhead       int       `json:"hoursAhead"`
	ForecastCPU      float64   `json:"forecastCpu"`
	ForecastMem      float64   `json:"forecastMem"`
	ForecastStorage  float64   `json:"forecastStorage"`
	ScaleRecommended bool      `json:"scaleRecommended"`
	Reason           string    `json:"reason"`
	Confidence       float64   `json:"confidence"`
	ForecastedAt     time.Time `json:"forecastedAt"`
}

type Forecaster struct {
	mu      sync.RWMutex
	samples map[string][]UtilizationSample
}

func NewForecaster() *Forecaster {
	return &Forecaster{
		samples: make(map[string][]UtilizationSample),
	}
}

func (f *Forecaster) AddSample(s UtilizationSample) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.samples[s.ResourceID] = append(f.samples[s.ResourceID], s)

	// Cap at 168 samples (7 days hourly)
	if len(f.samples[s.ResourceID]) > 168 {
		f.samples[s.ResourceID] = f.samples[s.ResourceID][1:]
	}
}

func (f *Forecaster) Forecast(resourceID string, hoursAhead int) (*ScaleForecast, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	samples, ok := f.samples[resourceID]
	if !ok || len(samples) == 0 {
		return &ScaleForecast{
			ResourceID:       resourceID,
			HoursAhead:       hoursAhead,
			ForecastCPU:      50.0,
			ForecastMem:      50.0,
			ForecastStorage:  50.0,
			ScaleRecommended: false,
			Reason:           "No data available",
			Confidence:       0.0,
			ForecastedAt:     time.Now(),
		}, nil
	}

	// Use last N samples (N = min(24, len(samples)))
	n := len(samples)
	if n > 24 {
		n = 24
	}

	start := len(samples) - n
	lastN := samples[start:]

	// Compute simple averages
	var sumCPU, sumMem, sumStorage float64
	for _, s := range lastN {
		sumCPU += s.CPUPercent
		sumMem += s.MemPercent
		sumStorage += s.StoragePct
	}

	avgCPU := sumCPU / float64(n)
	avgMem := sumMem / float64(n)
	avgStorage := sumStorage / float64(n)

	// Simple trend: if latest is higher than average, increase forecast
	latest := lastN[len(lastN)-1]
	trendCPU := 1.0
	if latest.CPUPercent > avgCPU {
		trendCPU = 1.05
	}
	trendMem := 1.0
	if latest.MemPercent > avgMem {
		trendMem = 1.05
	}
	trendStorage := 1.0
	if latest.StoragePct > avgStorage {
		trendStorage = 1.05
	}

	forecastCPU := avgCPU * trendCPU * (1.0 + float64(hoursAhead)*0.01)
	forecastMem := avgMem * trendMem * (1.0 + float64(hoursAhead)*0.01)
	forecastStorage := avgStorage * trendStorage * (1.0 + float64(hoursAhead)*0.01)

	scaleRecommended := forecastCPU > 70 || forecastMem > 70 || forecastStorage > 70

	reason := "Within normal range"
	if scaleRecommended {
		reason = "Forecasted utilization exceeds 70% threshold"
	}

	confidence := float64(n) / 24.0 * 80.0
	if confidence > 80 {
		confidence = 80.0
	}

	return &ScaleForecast{
		ResourceID:       resourceID,
		HoursAhead:       hoursAhead,
		ForecastCPU:      forecastCPU,
		ForecastMem:      forecastMem,
		ForecastStorage:  forecastStorage,
		ScaleRecommended: scaleRecommended,
		Reason:           reason,
		Confidence:       confidence,
		ForecastedAt:     time.Now(),
	}, nil
}

func (f *Forecaster) ListResources() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	var resources []string
	for rid := range f.samples {
		resources = append(resources, rid)
	}
	return resources
}
