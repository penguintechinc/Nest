package main

import (
	"fmt"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"
)

func getTestLogger() *zap.Logger {
	logger, _ := zap.NewDevelopment()
	return logger
}

func getTestAuditLogger(t *testing.T) *AuditLogger {
	tmpDir := t.TempDir()
	dbPath := fmt.Sprintf("%s/test-audit.db", tmpDir)

	os.Setenv("DB_TYPE", "sqlite")
	os.Setenv("DB_NAME", dbPath)

	logger := getTestLogger()
	defer logger.Sync()

	auditLogger, err := NewAuditLogger(logger)
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}

	t.Cleanup(func() {
		auditLogger.Close()
		os.Unsetenv("DB_TYPE")
		os.Unsetenv("DB_NAME")
	})

	return auditLogger
}

func TestNewAuditLogger(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()

	tmpDir := t.TempDir()
	dbPath := fmt.Sprintf("%s/test-audit.db", tmpDir)

	os.Setenv("DB_TYPE", "sqlite")
	os.Setenv("DB_NAME", dbPath)
	defer os.Unsetenv("DB_TYPE")
	defer os.Unsetenv("DB_NAME")

	auditLogger, err := NewAuditLogger(logger)
	if err != nil {
		t.Fatalf("NewAuditLogger returned error: %v", err)
	}
	defer auditLogger.Close()

	if auditLogger == nil {
		t.Fatal("NewAuditLogger returned nil")
	}
	if auditLogger.store == nil {
		t.Fatal("store is nil")
	}
	if auditLogger.logger != logger {
		t.Fatal("logger not set correctly")
	}
}

func TestAppend_ValidEvent(t *testing.T) {
	auditLogger := getTestAuditLogger(t)

	event := &AuditEvent{
		Tenant:   "tenant-1",
		Actor:    "user-1",
		Action:   "create",
		Resource: "resource-1",
		Outcome:  "success",
	}

	err := auditLogger.Append(event)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	results := auditLogger.Query(AuditFilter{Tenant: "tenant-1", Limit: 100})
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestAppend_GeneratesID(t *testing.T) {
	auditLogger := getTestAuditLogger(t)

	event := &AuditEvent{
		Tenant:   "tenant-1",
		Actor:    "user-1",
		Action:   "create",
		Resource: "resource-1",
		Outcome:  "success",
	}

	err := auditLogger.Append(event)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if event.ID == "" {
		t.Fatal("ID was not generated")
	}

	if len(event.ID) < 4 || event.ID[:4] != "evt-" {
		t.Errorf("expected ID format evt-*, got %s", event.ID)
	}
}

func TestAppend_GeneratesTimestamp(t *testing.T) {
	auditLogger := getTestAuditLogger(t)

	event := &AuditEvent{
		Tenant:   "tenant-1",
		Actor:    "user-1",
		Action:   "create",
		Resource: "resource-1",
		Outcome:  "success",
	}

	before := time.Now()
	err := auditLogger.Append(event)
	after := time.Now()

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if event.Timestamp.IsZero() {
		t.Fatal("Timestamp was not set")
	}

	if event.Timestamp.Before(before) || event.Timestamp.After(after.Add(1*time.Second)) {
		t.Errorf("Timestamp not in expected range: %v (expected between %v and %v)", event.Timestamp, before, after)
	}
}

func TestAppend_MissingTenant(t *testing.T) {
	auditLogger := getTestAuditLogger(t)

	event := &AuditEvent{
		Actor:    "user-1",
		Action:   "create",
		Resource: "resource-1",
		Outcome:  "success",
	}

	err := auditLogger.Append(event)
	if err == nil {
		t.Fatal("expected error for missing tenant")
	}

	if err.Error() != "tenant is required" {
		t.Errorf("expected 'tenant is required', got '%s'", err.Error())
	}
}

func TestAppend_MissingActor(t *testing.T) {
	auditLogger := getTestAuditLogger(t)

	event := &AuditEvent{
		Tenant:   "tenant-1",
		Action:   "create",
		Resource: "resource-1",
		Outcome:  "success",
	}

	err := auditLogger.Append(event)
	if err == nil {
		t.Fatal("expected error for missing actor")
	}

	if err.Error() != "actor is required" {
		t.Errorf("expected 'actor is required', got '%s'", err.Error())
	}
}

func TestAppend_MissingAction(t *testing.T) {
	auditLogger := getTestAuditLogger(t)

	event := &AuditEvent{
		Tenant:   "tenant-1",
		Actor:    "user-1",
		Resource: "resource-1",
		Outcome:  "success",
	}

	err := auditLogger.Append(event)
	if err == nil {
		t.Fatal("expected error for missing action")
	}

	if err.Error() != "action is required" {
		t.Errorf("expected 'action is required', got '%s'", err.Error())
	}
}

func TestAppend_MissingResource(t *testing.T) {
	auditLogger := getTestAuditLogger(t)

	event := &AuditEvent{
		Tenant:  "tenant-1",
		Actor:   "user-1",
		Action:  "create",
		Outcome: "success",
	}

	err := auditLogger.Append(event)
	if err == nil {
		t.Fatal("expected error for missing resource")
	}

	if err.Error() != "resource is required" {
		t.Errorf("expected 'resource is required', got '%s'", err.Error())
	}
}

func TestAppend_MissingOutcome(t *testing.T) {
	auditLogger := getTestAuditLogger(t)

	event := &AuditEvent{
		Tenant:   "tenant-1",
		Actor:    "user-1",
		Action:   "create",
		Resource: "resource-1",
	}

	err := auditLogger.Append(event)
	if err == nil {
		t.Fatal("expected error for missing outcome")
	}

	if err.Error() != "outcome is required" {
		t.Errorf("expected 'outcome is required', got '%s'", err.Error())
	}
}

func TestQuery_FilterByTenant(t *testing.T) {
	auditLogger := getTestAuditLogger(t)

	auditLogger.Append(&AuditEvent{
		Tenant:   "tenant-1",
		Actor:    "user-1",
		Action:   "create",
		Resource: "resource-1",
		Outcome:  "success",
	})

	auditLogger.Append(&AuditEvent{
		Tenant:   "tenant-2",
		Actor:    "user-2",
		Action:   "delete",
		Resource: "resource-2",
		Outcome:  "success",
	})

	filter := AuditFilter{Tenant: "tenant-1", Limit: 100}
	results := auditLogger.Query(filter)

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}

	if results[0].Tenant != "tenant-1" {
		t.Errorf("expected tenant-1, got %s", results[0].Tenant)
	}
}

func TestQuery_FilterByAction(t *testing.T) {
	auditLogger := getTestAuditLogger(t)

	auditLogger.Append(&AuditEvent{
		Tenant:   "tenant-1",
		Actor:    "user-1",
		Action:   "create",
		Resource: "resource-1",
		Outcome:  "success",
	})

	auditLogger.Append(&AuditEvent{
		Tenant:   "tenant-1",
		Actor:    "user-1",
		Action:   "delete",
		Resource: "resource-2",
		Outcome:  "success",
	})

	filter := AuditFilter{Action: "create", Limit: 100}
	results := auditLogger.Query(filter)

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}

	if results[0].Action != "create" {
		t.Errorf("expected action 'create', got '%s'", results[0].Action)
	}
}

func TestQuery_FilterByResource(t *testing.T) {
	auditLogger := getTestAuditLogger(t)

	auditLogger.Append(&AuditEvent{
		Tenant:   "tenant-1",
		Actor:    "user-1",
		Action:   "create",
		Resource: "resource-1",
		Outcome:  "success",
	})

	auditLogger.Append(&AuditEvent{
		Tenant:   "tenant-1",
		Actor:    "user-1",
		Action:   "read",
		Resource: "resource-2",
		Outcome:  "success",
	})

	filter := AuditFilter{Resource: "resource-1", Limit: 100}
	results := auditLogger.Query(filter)

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}

	if results[0].Resource != "resource-1" {
		t.Errorf("expected resource 'resource-1', got '%s'", results[0].Resource)
	}
}

func TestQuery_FilterByOutcome(t *testing.T) {
	auditLogger := getTestAuditLogger(t)

	auditLogger.Append(&AuditEvent{
		Tenant:   "tenant-1",
		Actor:    "user-1",
		Action:   "create",
		Resource: "resource-1",
		Outcome:  "success",
	})

	auditLogger.Append(&AuditEvent{
		Tenant:   "tenant-1",
		Actor:    "user-1",
		Action:   "delete",
		Resource: "resource-2",
		Outcome:  "denied",
	})

	filter := AuditFilter{Outcome: "denied", Limit: 100}
	results := auditLogger.Query(filter)

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}

	if results[0].Outcome != "denied" {
		t.Errorf("expected outcome 'denied', got '%s'", results[0].Outcome)
	}
}

func TestQuery_FilterByActor(t *testing.T) {
	auditLogger := getTestAuditLogger(t)

	auditLogger.Append(&AuditEvent{
		Tenant:   "tenant-1",
		Actor:    "user-1",
		Action:   "create",
		Resource: "resource-1",
		Outcome:  "success",
	})

	auditLogger.Append(&AuditEvent{
		Tenant:   "tenant-1",
		Actor:    "user-2",
		Action:   "delete",
		Resource: "resource-2",
		Outcome:  "success",
	})

	filter := AuditFilter{Actor: "user-1", Limit: 100}
	results := auditLogger.Query(filter)

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}

	if results[0].Actor != "user-1" {
		t.Errorf("expected actor 'user-1', got '%s'", results[0].Actor)
	}
}

func TestQuery_LimitAndOffset(t *testing.T) {
	auditLogger := getTestAuditLogger(t)

	for i := 1; i <= 5; i++ {
		auditLogger.Append(&AuditEvent{
			Tenant:   "tenant-1",
			Actor:    fmt.Sprintf("user-%d", i),
			Action:   "create",
			Resource: fmt.Sprintf("resource-%d", i),
			Outcome:  "success",
		})
	}

	filter := AuditFilter{Limit: 2}
	results := auditLogger.Query(filter)

	if len(results) != 2 {
		t.Errorf("expected 2 results with limit 2, got %d", len(results))
	}

	filter = AuditFilter{Offset: 3, Limit: 100}
	results = auditLogger.Query(filter)

	if len(results) != 2 {
		t.Errorf("expected 2 results with offset 3, got %d", len(results))
	}

	filter = AuditFilter{Offset: 1, Limit: 2}
	results = auditLogger.Query(filter)

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestQuery_DefaultLimit(t *testing.T) {
	auditLogger := getTestAuditLogger(t)

	for i := 1; i <= 150; i++ {
		auditLogger.Append(&AuditEvent{
			Tenant:   "tenant-1",
			Actor:    "user-1",
			Action:   "create",
			Resource: fmt.Sprintf("resource-%d", i),
			Outcome:  "success",
		})
	}

	filter := AuditFilter{}
	results := auditLogger.Query(filter)

	if len(results) != 100 {
		t.Errorf("expected default limit 100, got %d", len(results))
	}
}

func TestQuery_MaxLimit(t *testing.T) {
	auditLogger := getTestAuditLogger(t)

	for i := 1; i <= 150; i++ {
		auditLogger.Append(&AuditEvent{
			Tenant:   "tenant-1",
			Actor:    "user-1",
			Action:   "create",
			Resource: fmt.Sprintf("resource-%d", i),
			Outcome:  "success",
		})
	}

	filter := AuditFilter{Limit: 2000}
	results := auditLogger.Query(filter)

	if len(results) != 150 {
		t.Errorf("expected 150 results (total available), got %d", len(results))
	}
}

func TestQuery_OffsetOutOfRange(t *testing.T) {
	auditLogger := getTestAuditLogger(t)

	auditLogger.Append(&AuditEvent{
		Tenant:   "tenant-1",
		Actor:    "user-1",
		Action:   "create",
		Resource: "resource-1",
		Outcome:  "success",
	})

	filter := AuditFilter{Offset: 10, Limit: 100}
	results := auditLogger.Query(filter)

	if len(results) != 0 {
		t.Errorf("expected 0 results with offset out of range, got %d", len(results))
	}
}

func TestQuery_TimeRange(t *testing.T) {
	auditLogger := getTestAuditLogger(t)

	now := time.Now()
	pastTime := now.Add(-1 * time.Hour)
	futureTime := now.Add(1 * time.Hour)

	event1 := &AuditEvent{
		Tenant:    "tenant-1",
		Actor:     "user-1",
		Action:    "create",
		Resource:  "resource-1",
		Outcome:   "success",
		Timestamp: pastTime,
	}
	auditLogger.Append(event1)

	event2 := &AuditEvent{
		Tenant:    "tenant-1",
		Actor:     "user-2",
		Action:    "delete",
		Resource:  "resource-2",
		Outcome:   "success",
		Timestamp: futureTime,
	}
	auditLogger.Append(event2)

	filter := AuditFilter{StartTime: now, Limit: 100}
	results := auditLogger.Query(filter)

	if len(results) != 1 {
		t.Errorf("expected 1 result with startTime filter, got %d", len(results))
	}

	filter = AuditFilter{EndTime: now, Limit: 100}
	results = auditLogger.Query(filter)

	if len(results) != 1 {
		t.Errorf("expected 1 result with endTime filter, got %d", len(results))
	}
}

func TestQuery_MultipleFilters(t *testing.T) {
	auditLogger := getTestAuditLogger(t)

	auditLogger.Append(&AuditEvent{
		Tenant:   "tenant-1",
		Actor:    "user-1",
		Action:   "create",
		Resource: "resource-1",
		Outcome:  "success",
	})

	auditLogger.Append(&AuditEvent{
		Tenant:   "tenant-1",
		Actor:    "user-2",
		Action:   "delete",
		Resource: "resource-1",
		Outcome:  "denied",
	})

	filter := AuditFilter{
		Tenant:   "tenant-1",
		Action:   "create",
		Resource: "resource-1",
		Outcome:  "success",
		Limit:    100,
	}
	results := auditLogger.Query(filter)

	if len(results) != 1 {
		t.Errorf("expected 1 result with multiple filters, got %d", len(results))
	}

	if results[0].Actor != "user-1" {
		t.Errorf("expected user-1, got %s", results[0].Actor)
	}
}

func TestQuery_NoResults(t *testing.T) {
	auditLogger := getTestAuditLogger(t)

	auditLogger.Append(&AuditEvent{
		Tenant:   "tenant-1",
		Actor:    "user-1",
		Action:   "create",
		Resource: "resource-1",
		Outcome:  "success",
	})

	filter := AuditFilter{Tenant: "tenant-2", Limit: 100}
	results := auditLogger.Query(filter)

	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestQuery_EmptyLog(t *testing.T) {
	auditLogger := getTestAuditLogger(t)

	filter := AuditFilter{Limit: 100}
	results := auditLogger.Query(filter)

	if results == nil {
		t.Fatal("expected empty slice, got nil")
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestAppend_Concurrent(t *testing.T) {
	auditLogger := getTestAuditLogger(t)

	done := make(chan bool)

	for i := 1; i <= 10; i++ {
		go func(id int) {
			event := &AuditEvent{
				Tenant:   "tenant-1",
				Actor:    fmt.Sprintf("user-%d", id),
				Action:   "create",
				Resource: fmt.Sprintf("resource-%d", id),
				Outcome:  "success",
			}
			auditLogger.Append(event)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	results := auditLogger.Query(AuditFilter{Tenant: "tenant-1", Limit: 100})
	if len(results) != 10 {
		t.Errorf("expected 10 events, got %d", len(results))
	}
}

func TestAppend_WithDetails(t *testing.T) {
	auditLogger := getTestAuditLogger(t)

	event := &AuditEvent{
		Tenant:   "tenant-1",
		Actor:    "user-1",
		Action:   "create",
		Resource: "resource-1",
		Outcome:  "success",
		Details: map[string]interface{}{
			"ipAddress": "192.168.1.1",
			"userAgent": "Mozilla/5.0",
		},
	}

	err := auditLogger.Append(event)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	results := auditLogger.Query(AuditFilter{Tenant: "tenant-1", Limit: 100})
	if len(results) != 1 {
		t.Errorf("expected 1 event, got %d", len(results))
	}

	if results[0].Details["ipAddress"] != "192.168.1.1" {
		t.Error("details not preserved")
	}
}

func TestAppend_WithSourceIP(t *testing.T) {
	auditLogger := getTestAuditLogger(t)

	event := &AuditEvent{
		Tenant:   "tenant-1",
		Actor:    "user-1",
		Action:   "create",
		Resource: "resource-1",
		Outcome:  "success",
		SourceIP: "192.168.1.1",
	}

	err := auditLogger.Append(event)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	results := auditLogger.Query(AuditFilter{Tenant: "tenant-1", Limit: 100})
	if len(results) != 1 {
		t.Errorf("expected 1 event, got %d", len(results))
	}

	if results[0].SourceIP != "192.168.1.1" {
		t.Error("sourceIP not preserved")
	}
}
