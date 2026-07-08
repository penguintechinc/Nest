package main

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// AuditEvent represents a single audit log entry.
type AuditEvent struct {
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Tenant    string                 `json:"tenant"`
	Actor     string                 `json:"actor"`    // user_uuid from JWT
	Action    string                 `json:"action"`   // create, read, update, delete, login, logout
	Resource  string                 `json:"resource"` // resource ID or type
	Outcome   string                 `json:"outcome"`  // success, denied, error
	SourceIP  string                 `json:"sourceIp,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// AuditLogger is an append-only audit log backed by durable storage.
// Events are persisted to Postgres/MySQL/SQLite with a cryptographic hash chain
// for tamper-detection.
type AuditLogger struct {
	store  *DurableStore
	logger *zap.Logger
}

// NewAuditLogger creates a new AuditLogger backed by durable storage.
// It initializes a DurableStore from environment configuration.
func NewAuditLogger(logger *zap.Logger) (*AuditLogger, error) {
	store, err := NewDurableStore(logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize durable store: %w", err)
	}
	return &AuditLogger{
		store:  store,
		logger: logger,
	}, nil
}

// Append adds an event to the durable audit log.
// Returns error only on validation failure or database error.
func (a *AuditLogger) Append(event *AuditEvent) error {
	return a.store.Append(event)
}

// Query returns events matching the filter criteria from durable storage.
func (a *AuditLogger) Query(filter AuditFilter) []*AuditEvent {
	return a.store.Query(filter)
}

// ValidateIntegrity validates the integrity of the audit chain.
// Returns nil if valid, or an error describing any tampering detected.
func (a *AuditLogger) ValidateIntegrity(ctx context.Context) error {
	return a.store.ValidateIntegrity(ctx)
}

// Close closes the audit logger and underlying database connection.
func (a *AuditLogger) Close() error {
	if a.store != nil {
		return a.store.Close()
	}
	return nil
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
