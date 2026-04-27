package main

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// AuditEvent represents a single audit log entry.
type AuditEvent struct {
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Tenant    string                 `json:"tenant"`
	Actor     string                 `json:"actor"` // sub from JWT
	Action    string                 `json:"action"` // create, read, update, delete, login, logout
	Resource  string                 `json:"resource"` // resource ID or type
	Outcome   string                 `json:"outcome"` // success, denied, error
	SourceIP  string                 `json:"sourceIp,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// AuditLogger is an append-only in-memory audit log.
// Production: persisted to Postgres + archived to S3.
type AuditLogger struct {
	mu     sync.RWMutex
	events []*AuditEvent
	logger *zap.Logger
}

// NewAuditLogger creates a new AuditLogger.
func NewAuditLogger(logger *zap.Logger) *AuditLogger {
	return &AuditLogger{
		events: make([]*AuditEvent, 0),
		logger: logger,
	}
}

// Append adds an event to the log. Returns error only on validation failure.
func (a *AuditLogger) Append(event *AuditEvent) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Generate ID if not provided
	if event.ID == "" {
		event.ID = fmt.Sprintf("evt-%d", time.Now().UnixNano())
	}

	// Set timestamp if not provided
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Validation: required fields
	if event.Tenant == "" {
		return fmt.Errorf("tenant is required")
	}
	if event.Actor == "" {
		return fmt.Errorf("actor is required")
	}
	if event.Action == "" {
		return fmt.Errorf("action is required")
	}
	if event.Resource == "" {
		return fmt.Errorf("resource is required")
	}
	if event.Outcome == "" {
		return fmt.Errorf("outcome is required")
	}

	a.events = append(a.events, event)
	a.logger.Debug("audit event appended", zap.String("id", event.ID), zap.String("action", event.Action))

	return nil
}

// Query returns events matching the filter criteria.
func (a *AuditLogger) Query(filter AuditFilter) []*AuditEvent {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Set default/max limit
	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	var results []*AuditEvent

	for _, event := range a.events {
		// Apply filters
		if filter.Tenant != "" && event.Tenant != filter.Tenant {
			continue
		}
		if filter.Actor != "" && event.Actor != filter.Actor {
			continue
		}
		if filter.Action != "" && event.Action != filter.Action {
			continue
		}
		if filter.Resource != "" && event.Resource != filter.Resource {
			continue
		}
		if filter.Outcome != "" && event.Outcome != filter.Outcome {
			continue
		}
		if !filter.StartTime.IsZero() && event.Timestamp.Before(filter.StartTime) {
			continue
		}
		if !filter.EndTime.IsZero() && event.Timestamp.After(filter.EndTime) {
			continue
		}

		results = append(results, event)
	}

	// Apply offset and limit
	if filter.Offset >= len(results) {
		return []*AuditEvent{}
	}

	end := filter.Offset + int(limit)
	if end > len(results) {
		end = len(results)
	}

	return results[filter.Offset:end]
}

// AuditFilter defines query criteria for audit events.
type AuditFilter struct {
	Tenant    string
	Actor     string
	Action    string
	Resource  string
	Outcome   string
	StartTime time.Time
	EndTime   time.Time
	Limit     int // default 100, max 1000
	Offset    int
}
