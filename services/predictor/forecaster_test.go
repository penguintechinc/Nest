package main

import (
	"testing"
	"time"
)

func TestNewForecaster(t *testing.T) {
	f := NewForecaster()
	if f == nil {
		t.Fatal("NewForecaster returned nil")
	}
	if f.samples == nil {
		t.Fatal("samples map is nil")
	}
	if len(f.samples) != 0 {
		t.Errorf("expected empty samples, got %d", len(f.samples))
	}
}

func TestAddSample(t *testing.T) {
	f := NewForecaster()
	s := UtilizationSample{
		ResourceID: "res1",
		Timestamp:  time.Now(),
		CPUPercent: 50.0,
		MemPercent: 60.0,
		StoragePct: 70.0,
	}
	f.AddSample(s)

	if len(f.samples) != 1 {
		t.Errorf("expected 1 resource, got %d", len(f.samples))
	}

	if samples, ok := f.samples["res1"]; !ok {
		t.Fatal("expected samples for res1")
	} else if len(samples) != 1 {
		t.Errorf("expected 1 sample, got %d", len(samples))
	} else if samples[0].CPUPercent != 50.0 {
		t.Errorf("expected CPUPercent 50.0, got %f", samples[0].CPUPercent)
	}
}

func TestAddSampleCapAt168(t *testing.T) {
	f := NewForecaster()

	// Add 300 samples
	for i := 0; i < 300; i++ {
		s := UtilizationSample{
			ResourceID: "res1",
			Timestamp:  time.Now().Add(time.Duration(i) * time.Hour),
			CPUPercent: float64(i % 100),
			MemPercent: float64((i + 10) % 100),
			StoragePct: float64((i + 20) % 100),
		}
		f.AddSample(s)
	}

	if len(f.samples["res1"]) != 168 {
		t.Errorf("expected 168 samples after cap, got %d", len(f.samples["res1"]))
	}

	// Verify oldest samples were removed (should start from sample 300+)
	expected := float64((300 + 132) % 100)
	if f.samples["res1"][0].CPUPercent != expected {
		t.Errorf("expected first sample value around 32, got %f", f.samples["res1"][0].CPUPercent)
	}
}

func TestForecastNoData(t *testing.T) {
	f := NewForecaster()

	forecast, err := f.Forecast("nonexistent", 24)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if forecast == nil {
		t.Fatal("expected forecast to be non-nil")
	}
	if forecast.ForecastCPU != 50.0 {
		t.Errorf("expected default ForecastCPU 50.0, got %f", forecast.ForecastCPU)
	}
	if forecast.ScaleRecommended {
		t.Errorf("expected ScaleRecommended=false for no data, got %v", forecast.ScaleRecommended)
	}
	if forecast.Confidence != 0.0 {
		t.Errorf("expected Confidence 0.0, got %f", forecast.Confidence)
	}
}

func TestForecastWithSamples(t *testing.T) {
	f := NewForecaster()

	// Add samples with 50% CPU
	for i := 0; i < 10; i++ {
		s := UtilizationSample{
			ResourceID: "res1",
			Timestamp:  time.Now().Add(time.Duration(i) * time.Hour),
			CPUPercent: 50.0,
			MemPercent: 60.0,
			StoragePct: 70.0,
		}
		f.AddSample(s)
	}

	forecast, err := f.Forecast("res1", 24)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if forecast == nil {
		t.Fatal("expected forecast to be non-nil")
	}

	// With 50% usage and hoursAhead=24, forecast should be around 50 * 1.0 * 1.24 = 62
	if forecast.ForecastCPU < 55 || forecast.ForecastCPU > 75 {
		t.Errorf("expected ForecastCPU around 62, got %f", forecast.ForecastCPU)
	}
}

func TestForecastScaleRecommendedHigh(t *testing.T) {
	f := NewForecaster()

	// Add samples with 80% CPU
	for i := 0; i < 24; i++ {
		s := UtilizationSample{
			ResourceID: "res1",
			Timestamp:  time.Now().Add(time.Duration(i) * time.Hour),
			CPUPercent: 80.0,
			MemPercent: 50.0,
			StoragePct: 50.0,
		}
		f.AddSample(s)
	}

	forecast, err := f.Forecast("res1", 0)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if !forecast.ScaleRecommended {
		t.Errorf("expected ScaleRecommended=true for 80 percent CPU, got %v", forecast.ScaleRecommended)
	}
	if forecast.ForecastCPU < 70 {
		t.Errorf("expected ForecastCPU > 70 (got %f)", forecast.ForecastCPU)
	}
}

func TestForecastScaleRecommendedLow(t *testing.T) {
	f := NewForecaster()

	// Add samples with 40% CPU
	for i := 0; i < 24; i++ {
		s := UtilizationSample{
			ResourceID: "res1",
			Timestamp:  time.Now().Add(time.Duration(i) * time.Hour),
			CPUPercent: 40.0,
			MemPercent: 30.0,
			StoragePct: 20.0,
		}
		f.AddSample(s)
	}

	forecast, err := f.Forecast("res1", 0)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if forecast.ScaleRecommended {
		t.Errorf("expected ScaleRecommended=false for 40 percent CPU, got %v", forecast.ScaleRecommended)
	}
}

func TestForecastTrendUpIncreasesForcast(t *testing.T) {
	f := NewForecaster()

	// Add increasing samples (trending up)
	for i := 0; i < 10; i++ {
		s := UtilizationSample{
			ResourceID: "res1",
			Timestamp:  time.Now().Add(time.Duration(i) * time.Hour),
			CPUPercent: 40.0 + float64(i)*2.0, // 40, 42, 44, ..., 58
			MemPercent: 50.0,
			StoragePct: 50.0,
		}
		f.AddSample(s)
	}

	forecast, err := f.Forecast("res1", 0)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Latest is 58, avg is ~49, so trend should increase forecast
	if forecast.ForecastCPU < 49 {
		t.Errorf("expected upward trend to increase forecast, got %f", forecast.ForecastCPU)
	}
}

func TestForecastConfidenceFormula(t *testing.T) {
	f := NewForecaster()

	// Test with 2 samples: confidence = 2/24 * 80 = 6.67
	for i := 0; i < 2; i++ {
		s := UtilizationSample{
			ResourceID: "res1",
			Timestamp:  time.Now().Add(time.Duration(i) * time.Hour),
			CPUPercent: 50.0,
			MemPercent: 50.0,
			StoragePct: 50.0,
		}
		f.AddSample(s)
	}

	forecast, _ := f.Forecast("res1", 0)
	expected := float64(2) / 24.0 * 80.0
	if forecast.Confidence < expected-0.1 || forecast.Confidence > expected+0.1 {
		t.Errorf("expected confidence ~%.2f, got %.2f", expected, forecast.Confidence)
	}
}

func TestForecastConfidenceCappedAt80(t *testing.T) {
	f := NewForecaster()

	// Add 24+ samples (would give > 80% confidence)
	for i := 0; i < 30; i++ {
		s := UtilizationSample{
			ResourceID: "res1",
			Timestamp:  time.Now().Add(time.Duration(i) * time.Hour),
			CPUPercent: 50.0,
			MemPercent: 50.0,
			StoragePct: 50.0,
		}
		f.AddSample(s)
	}

	forecast, _ := f.Forecast("res1", 0)
	if forecast.Confidence != 80.0 {
		t.Errorf("expected confidence capped at 80.0, got %f", forecast.Confidence)
	}
}

func TestForecastMultipleResources(t *testing.T) {
	f := NewForecaster()

	// Add samples for multiple resources
	for rid := 1; rid <= 5; rid++ {
		resID := "res" + string(rune(rid+48))
		for i := 0; i < 10; i++ {
			s := UtilizationSample{
				ResourceID: resID,
				Timestamp:  time.Now().Add(time.Duration(i) * time.Hour),
				CPUPercent: float64(rid * 10),
				MemPercent: float64(rid * 10),
				StoragePct: float64(rid * 10),
			}
			f.AddSample(s)
		}
	}

	if len(f.samples) != 5 {
		t.Errorf("expected 5 resources, got %d", len(f.samples))
	}

	// Verify each resource has correct forecast
	for rid := 1; rid <= 5; rid++ {
		resID := "res" + string(rune(rid+48))
		forecast, _ := f.Forecast(resID, 0)
		if forecast.ResourceID != resID {
			t.Errorf("expected ResourceID %s, got %s", resID, forecast.ResourceID)
		}
	}
}

func TestListResources(t *testing.T) {
	f := NewForecaster()

	// Add samples for 3 resources
	for i := 1; i <= 3; i++ {
		resID := "res" + string(rune(i+48))
		s := UtilizationSample{
			ResourceID: resID,
			Timestamp:  time.Now(),
			CPUPercent: 50.0,
			MemPercent: 50.0,
			StoragePct: 50.0,
		}
		f.AddSample(s)
	}

	resources := f.ListResources()
	if len(resources) != 3 {
		t.Errorf("expected 3 resources, got %d", len(resources))
	}

	// Verify all expected resources are present
	resMap := make(map[string]bool)
	for _, r := range resources {
		resMap[r] = true
	}
	for i := 1; i <= 3; i++ {
		resID := "res" + string(rune(i+48))
		if !resMap[resID] {
			t.Errorf("expected resource %s in list", resID)
		}
	}
}

func TestListResourcesEmpty(t *testing.T) {
	f := NewForecaster()
	resources := f.ListResources()
	if len(resources) != 0 {
		t.Errorf("expected empty resources list, got %d", len(resources))
	}
}

func TestForecastHoursAheadImpact(t *testing.T) {
	f := NewForecaster()

	// Add samples
	for i := 0; i < 10; i++ {
		s := UtilizationSample{
			ResourceID: "res1",
			Timestamp:  time.Now().Add(time.Duration(i) * time.Hour),
			CPUPercent: 50.0,
			MemPercent: 50.0,
			StoragePct: 50.0,
		}
		f.AddSample(s)
	}

	// Forecast 0 hours ahead
	forecast0, _ := f.Forecast("res1", 0)
	// Forecast 24 hours ahead
	forecast24, _ := f.Forecast("res1", 24)

	// With more hours ahead, forecast should be higher (due to trend multiplier)
	if forecast24.ForecastCPU <= forecast0.ForecastCPU {
		t.Errorf("expected forecast24 (%f) > forecast0 (%f)", forecast24.ForecastCPU, forecast0.ForecastCPU)
	}
}

func TestForecastMemoryAndStorage(t *testing.T) {
	f := NewForecaster()

	// Add samples with specific mem and storage values
	for i := 0; i < 10; i++ {
		s := UtilizationSample{
			ResourceID: "res1",
			Timestamp:  time.Now().Add(time.Duration(i) * time.Hour),
			CPUPercent: 30.0,
			MemPercent: 75.0,
			StoragePct: 85.0,
		}
		f.AddSample(s)
	}

	forecast, _ := f.Forecast("res1", 0)

	// Memory should trigger scale recommendation (> 70)
	if !forecast.ScaleRecommended {
		t.Errorf("expected ScaleRecommended=true for 75 percent memory, got %v", forecast.ScaleRecommended)
	}

	// All metrics should be present
	if forecast.ForecastCPU < 25 {
		t.Errorf("expected ForecastCPU ~30, got %f", forecast.ForecastCPU)
	}
	if forecast.ForecastMem < 70 {
		t.Errorf("expected ForecastMem ~75, got %f", forecast.ForecastMem)
	}
	if forecast.ForecastStorage < 80 {
		t.Errorf("expected ForecastStorage ~85, got %f", forecast.ForecastStorage)
	}
}

func TestForecastScenario24ConstantSamplesAt80Percent(t *testing.T) {
	f := NewForecaster()

	// Add 24 constant samples at 80%
	for i := 0; i < 24; i++ {
		s := UtilizationSample{
			ResourceID: "res1",
			Timestamp:  time.Now().Add(time.Duration(i) * time.Hour),
			CPUPercent: 80.0,
			MemPercent: 80.0,
			StoragePct: 80.0,
		}
		f.AddSample(s)
	}

	forecast, _ := f.Forecast("res1", 0)

	// ScaleRecommended should be true (all metrics > 70)
	if !forecast.ScaleRecommended {
		t.Errorf("expected ScaleRecommended=true for 80 percent utilization, got %v", forecast.ScaleRecommended)
	}

	// Confidence should be 80% (min(24/24*80, 80))
	if forecast.Confidence != 80.0 {
		t.Errorf("expected Confidence 80.0, got %f", forecast.Confidence)
	}

	if forecast.Reason != "Forecasted utilization exceeds 70% threshold" {
		t.Errorf("expected reason about threshold, got: %s", forecast.Reason)
	}
}

func TestForecastConcurrency(t *testing.T) {
	f := NewForecaster()

	// Add samples concurrently
	done := make(chan bool, 20)
	for i := 0; i < 20; i++ {
		go func(idx int) {
			resID := "res" + string(rune(idx%5+48))
			for j := 0; j < 10; j++ {
				s := UtilizationSample{
					ResourceID: resID,
					Timestamp:  time.Now().Add(time.Duration(j*1000+idx) * time.Millisecond),
					CPUPercent: float64(idx*5 + j),
					MemPercent: float64(idx*5 + j),
					StoragePct: float64(idx*5 + j),
				}
				f.AddSample(s)
			}
			done <- true
		}(i)
	}

	for i := 0; i < 20; i++ {
		<-done
	}

	if len(f.samples) != 5 {
		t.Errorf("expected 5 resources, got %d", len(f.samples))
	}

	// Verify forecasts work concurrently
	done2 := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			resID := "res" + string(rune(idx%5+48))
			f.Forecast(resID, 24)
			done2 <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done2
	}
}

func TestForecastTimestamp(t *testing.T) {
	f := NewForecaster()

	s := UtilizationSample{
		ResourceID: "res1",
		Timestamp:  time.Now(),
		CPUPercent: 50.0,
		MemPercent: 50.0,
		StoragePct: 50.0,
	}
	f.AddSample(s)

	forecast, _ := f.Forecast("res1", 0)

	if forecast.ForecastedAt.IsZero() {
		t.Fatal("expected ForecastedAt to be set")
	}
}

func TestForecastResourceIDInResult(t *testing.T) {
	f := NewForecaster()

	s := UtilizationSample{
		ResourceID: "test-resource-123",
		Timestamp:  time.Now(),
		CPUPercent: 50.0,
		MemPercent: 50.0,
		StoragePct: 50.0,
	}
	f.AddSample(s)

	forecast, _ := f.Forecast("test-resource-123", 24)

	if forecast.ResourceID != "test-resource-123" {
		t.Errorf("expected ResourceID test-resource-123, got %s", forecast.ResourceID)
	}
	if forecast.HoursAhead != 24 {
		t.Errorf("expected HoursAhead 24, got %d", forecast.HoursAhead)
	}
}

func TestForecastSingleSample(t *testing.T) {
	f := NewForecaster()

	s := UtilizationSample{
		ResourceID: "res1",
		Timestamp:  time.Now(),
		CPUPercent: 50.0,
		MemPercent: 50.0,
		StoragePct: 50.0,
	}
	f.AddSample(s)

	forecast, _ := f.Forecast("res1", 0)

	// With 1 sample: confidence = 1/24 * 80 = 3.33
	expectedConf := float64(1) / 24.0 * 80.0
	if forecast.Confidence < expectedConf-0.1 || forecast.Confidence > expectedConf+0.1 {
		t.Errorf("expected confidence ~%.2f, got %.2f", expectedConf, forecast.Confidence)
	}

	// Forecast should be based on single sample
	if forecast.ForecastCPU < 45 || forecast.ForecastCPU > 55 {
		t.Errorf("expected ForecastCPU ~50, got %f", forecast.ForecastCPU)
	}
}

func TestForecastReasonMessages(t *testing.T) {
	f := NewForecaster()

	// Test "Within normal range" reason
	for i := 0; i < 10; i++ {
		s := UtilizationSample{
			ResourceID: "res1",
			Timestamp:  time.Now().Add(time.Duration(i) * time.Hour),
			CPUPercent: 30.0,
			MemPercent: 30.0,
			StoragePct: 30.0,
		}
		f.AddSample(s)
	}

	forecast, _ := f.Forecast("res1", 0)
	if forecast.Reason != "Within normal range" {
		t.Errorf("expected reason 'Within normal range', got: %s", forecast.Reason)
	}

	// Test "exceeds 70% threshold" reason
	f2 := NewForecaster()
	for i := 0; i < 10; i++ {
		s := UtilizationSample{
			ResourceID: "res2",
			Timestamp:  time.Now().Add(time.Duration(i) * time.Hour),
			CPUPercent: 80.0,
			MemPercent: 80.0,
			StoragePct: 80.0,
		}
		f2.AddSample(s)
	}

	forecast2, _ := f2.Forecast("res2", 0)
	if forecast2.Reason != "Forecasted utilization exceeds 70% threshold" {
		t.Errorf("expected threshold reason, got: %s", forecast2.Reason)
	}
}
