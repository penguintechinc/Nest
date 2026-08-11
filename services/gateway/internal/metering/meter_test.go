package metering

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNewMeter(t *testing.T) {
	logger := zap.NewNop()
	m := NewMeter(logger)

	if m == nil {
		t.Error("NewMeter() returned nil")
	}

	if m.ch == nil {
		t.Error("NewMeter() did not initialize channel")
	}

	if m.events == nil {
		t.Error("NewMeter() did not initialize events slice")
	}
}

func TestMeter_Emit(t *testing.T) {
	logger := zap.NewNop()
	m := NewMeter(logger)
	defer m.Close()

	event := MeterEvent{
		TenantID:     "tenant1",
		ResourceType: "database",
		RequestPath:  "/api/v1/query",
		DurationMs:   100,
		DataBytes:    1024,
	}

	m.Emit(event)

	// Give drain loop time to process
	time.Sleep(50 * time.Millisecond)

	// Check that event was recorded
	summary := m.Summary("tenant1")
	if summary["total_requests"] != 1.0 {
		t.Errorf("Event not recorded")
	}

	if summary["total_tokens"] <= 0 {
		t.Errorf("TokenCount not calculated")
	}
}

func TestMeter_TokenCount_Calculation(t *testing.T) {
	logger := zap.NewNop()
	m := NewMeter(logger)
	defer m.Close()

	event := MeterEvent{
		TenantID:   "tenant1",
		DurationMs: 100,
		DataBytes:  0,
	}

	m.Emit(event)
	time.Sleep(10 * time.Millisecond)

	// TokenCount = durationMs * 0.001 + dataBytes / (1024*1024*1024) * 0.01
	// = 100 * 0.001 + 0
	// = 0.1
	summary := m.Summary("tenant1")
	if summary["total_tokens"] < 0.09 || summary["total_tokens"] > 0.11 {
		t.Errorf("TokenCount calculation wrong: %f", summary["total_tokens"])
	}
}

func TestMeter_Query_EmptyRange(t *testing.T) {
	logger := zap.NewNop()
	m := NewMeter(logger)
	defer m.Close()

	now := time.Now()
	event := MeterEvent{
		TenantID:    "tenant1",
		RequestPath: "/api/v1/query",
		DurationMs:  100,
	}

	m.Emit(event)
	time.Sleep(10 * time.Millisecond)

	// Query before event was emitted
	results := m.Query("tenant1", now.Add(-1*time.Hour), now.Add(-1*time.Minute))
	if len(results) != 0 {
		t.Errorf("Query should return 0 results for past time range, got %d", len(results))
	}
}

func TestMeter_Query_WithinRange(t *testing.T) {
	logger := zap.NewNop()
	m := NewMeter(logger)
	defer m.Close()

	before := time.Now()
	event := MeterEvent{
		TenantID:    "tenant1",
		RequestPath: "/api/v1/query",
		DurationMs:  100,
	}

	m.Emit(event)
	time.Sleep(10 * time.Millisecond)
	after := time.Now()

	// Query within time range
	results := m.Query("tenant1", before.Add(-1*time.Second), after.Add(1*time.Second))
	if len(results) != 1 {
		t.Errorf("Query should return 1 result, got %d", len(results))
	}
}

func TestMeter_Query_DifferentTenant(t *testing.T) {
	logger := zap.NewNop()
	m := NewMeter(logger)
	defer m.Close()

	event := MeterEvent{
		TenantID:    "tenant1",
		RequestPath: "/api/v1/query",
		DurationMs:  100,
	}

	m.Emit(event)
	time.Sleep(10 * time.Millisecond)

	// Query for different tenant
	results := m.Query("tenant2", time.Now().Add(-1*time.Hour), time.Now().Add(1*time.Hour))
	if len(results) != 0 {
		t.Errorf("Query should return 0 results for different tenant, got %d", len(results))
	}
}

func TestMeter_Summary(t *testing.T) {
	logger := zap.NewNop()
	m := NewMeter(logger)
	defer m.Close()

	// Emit 3 events
	for i := 0; i < 3; i++ {
		event := MeterEvent{
			TenantID:   "tenant1",
			DurationMs: 50,
			DataBytes:  512,
		}
		m.Emit(event)
	}

	time.Sleep(50 * time.Millisecond)

	summary := m.Summary("tenant1")

	if summary["total_requests"] != 3.0 {
		t.Errorf("total_requests = %f, want 3.0", summary["total_requests"])
	}

	if summary["total_tokens"] <= 0 {
		t.Errorf("total_tokens should be > 0, got %f", summary["total_tokens"])
	}
}

func TestMeter_Summary_NoEvents(t *testing.T) {
	logger := zap.NewNop()
	m := NewMeter(logger)
	defer m.Close()

	summary := m.Summary("tenant1")

	if summary["total_requests"] != 0.0 {
		t.Errorf("total_requests = %f, want 0.0", summary["total_requests"])
	}

	if summary["total_tokens"] != 0.0 {
		t.Errorf("total_tokens = %f, want 0.0", summary["total_tokens"])
	}
}

func TestMeterEvent_Structure(t *testing.T) {
	now := time.Now()
	event := MeterEvent{
		TenantID:     "tenant1",
		ResourceType: "database",
		RequestPath:  "/api/v1/query",
		DurationMs:   100,
		DataBytes:    1024,
		TokenCount:   1.5,
		Timestamp:    now,
	}

	if event.TenantID != "tenant1" {
		t.Errorf("TenantID = %q, want tenant1", event.TenantID)
	}
	if event.ResourceType != "database" {
		t.Errorf("ResourceType = %q, want database", event.ResourceType)
	}
	if event.RequestPath != "/api/v1/query" {
		t.Errorf("RequestPath = %q, want /api/v1/query", event.RequestPath)
	}
	if event.DurationMs != 100 {
		t.Errorf("DurationMs = %d, want 100", event.DurationMs)
	}
	if event.DataBytes != 1024 {
		t.Errorf("DataBytes = %d, want 1024", event.DataBytes)
	}
	if event.TokenCount != 1.5 {
		t.Errorf("TokenCount = %f, want 1.5", event.TokenCount)
	}
	if !event.Timestamp.Equal(now) {
		t.Error("Timestamp not set correctly")
	}
}

func TestMeter_Close(t *testing.T) {
	logger := zap.NewNop()
	m := NewMeter(logger)

	// Should not panic
	m.Close()
}
