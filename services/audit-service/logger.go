package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/penguintechinc/nest/shared/database"
	"go.uber.org/zap"
	"gorm.io/datatypes"
)

// AuditEvent represents a single audit log entry.
type AuditEvent struct {
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Tenant    string                 `json:"tenant"`
	Actor     string                 `json:"actor"`   // sub from JWT
	Action    string                 `json:"action"`  // create, read, update, delete, login, logout
	Resource  string                 `json:"resource"` // resource ID or type
	Outcome   string                 `json:"outcome"`  // success, denied, error
	SourceIP  string                 `json:"sourceIp,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// AuditLogger is a persistent audit log powered by PenguinDAL.
type AuditLogger struct {
	dal    *database.PenguinDAL
	logger *zap.Logger
}

// NewAuditLogger creates a new AuditLogger.
func NewAuditLogger(dal *database.PenguinDAL, logger *zap.Logger) *AuditLogger {
	// Ensure table exists
	dal.DefineTable(&database.AuditLog{})
	return &AuditLogger{
		dal:    dal,
		logger: logger,
	}
}

// Append adds an event to the log.
func (a *AuditLogger) Append(event *AuditEvent) error {
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

	// Map to database model
	detailsJSON, _ := json.Marshal(event.Details)
	logEntry := &database.AuditLog{
		Action:       event.Action,
		ResourceType: event.Resource,
		Details:      datatypes.JSON(detailsJSON),
		IPAddress:    event.SourceIP,
		Timestamp:    event.Timestamp,
	}
	
	if logEntry.Timestamp.IsZero() {
		logEntry.Timestamp = time.Now()
	}
	event.Timestamp = logEntry.Timestamp

	if err := a.dal.Insert(logEntry); err != nil {
		a.logger.Error("failed to persist audit log", zap.Error(err))
		return err
	}

	event.ID = fmt.Sprintf("%d", logEntry.ID)
	a.logger.Debug("audit event persisted", zap.Uint("id", logEntry.ID), zap.String("action", event.Action))

	return nil
}

// Query returns events matching the filter criteria.
func (a *AuditLogger) Query(filter AuditFilter) []*AuditEvent {
	query := a.dal.Query().Model(&database.AuditLog{})

	if filter.Action != "" {
		query = query.Where("action = ?", filter.Action)
	}
	if filter.Resource != "" {
		query = query.Where("resource_type = ?", filter.Resource)
	}
	if !filter.StartTime.IsZero() {
		query = query.Where("timestamp >= ?", filter.StartTime)
	}
	if !filter.EndTime.IsZero() {
		query = query.Where("timestamp <= ?", filter.EndTime)
	}

	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	query = query.Limit(limit).Offset(filter.Offset).Order("timestamp desc")

	var entries []database.AuditLog
	if err := query.Find(&entries).Error; err != nil {
		a.logger.Error("failed to query audit logs", zap.Error(err))
		return []*AuditEvent{}
	}

	results := make([]*AuditEvent, len(entries))
	for i, entry := range entries {
		var details map[string]interface{}
		json.Unmarshal(entry.Details, &details)

		results[i] = &AuditEvent{
			ID:        fmt.Sprintf("%d", entry.ID),
			Timestamp: entry.Timestamp,
			Action:    entry.Action,
			Resource:  entry.ResourceType,
			SourceIP:  entry.IPAddress,
			Details:   details,
		}
	}

	return results
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
