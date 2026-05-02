package metering

import (
	"sync"
	"time"

	"go.uber.org/zap"
)

// MeterEvent records a single API request cost
type MeterEvent struct {
	TenantID     string    `json:"tenantId"`
	ResourceType string    `json:"resourceType,omitempty"`
	RequestPath  string    `json:"requestPath"`
	DurationMs   int64     `json:"durationMs"`
	DataBytes    int64     `json:"dataBytes,omitempty"`
	TokenCount   float64   `json:"tokenCount"`
	Timestamp    time.Time `json:"timestamp"`
}

// Meter emits token usage events (fire-and-forget, never blocks request path)
type Meter struct {
	ch     chan MeterEvent
	events []MeterEvent
	mu     sync.Mutex
	logger *zap.Logger
}

// NewMeter creates a Meter with a 1000-event buffered channel
// and starts a background drain goroutine
func NewMeter(logger *zap.Logger) *Meter {
	m := &Meter{
		ch:     make(chan MeterEvent, 1000),
		events: make([]MeterEvent, 0),
		logger: logger,
	}
	go m.drainLoop()
	return m
}

// Emit sends a MeterEvent (non-blocking; drops on full channel)
// TokenCount = durationMs * 0.001 + dataBytes / (1024*1024*1024) * 0.01
func (m *Meter) Emit(event MeterEvent) {
	event.TokenCount = float64(event.DurationMs)*0.001 + float64(event.DataBytes)/(1024*1024*1024)*0.01
	event.Timestamp = time.Now()
	select {
	case m.ch <- event:
	default:
		// Channel full, drop event (non-blocking)
	}
}

// drainLoop reads from ch and appends to events slice, logs at debug level
func (m *Meter) drainLoop() {
	for event := range m.ch {
		m.mu.Lock()
		m.events = append(m.events, event)
		m.mu.Unlock()
		m.logger.Debug("meter event",
			zap.String("tenant", event.TenantID),
			zap.String("path", event.RequestPath),
			zap.Float64("tokens", event.TokenCount),
		)
	}
}

// Query returns events for tenant in time range
func (m *Meter) Query(tenantID string, start, end time.Time) []MeterEvent {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []MeterEvent
	for _, e := range m.events {
		if e.TenantID == tenantID && e.Timestamp.After(start) && e.Timestamp.Before(end) {
			result = append(result, e)
		}
	}
	return result
}

// Summary returns aggregated token usage for tenant
func (m *Meter) Summary(tenantID string) map[string]float64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	totalTokens := 0.0
	totalRequests := 0.0
	for _, e := range m.events {
		if e.TenantID == tenantID {
			totalTokens += e.TokenCount
			totalRequests += 1.0
		}
	}
	return map[string]float64{
		"total_tokens":   totalTokens,
		"total_requests": totalRequests,
	}
}

// Close closes the drain channel
func (m *Meter) Close() {
	close(m.ch)
}
