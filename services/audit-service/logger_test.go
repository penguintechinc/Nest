package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/penguintechinc/nest/shared/database"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func getTestLogger() *zap.Logger {
	logger, _ := zap.NewDevelopment()
	return logger
}

var (
	testDB *gorm.DB
)

func getTestDAL() *database.PenguinDAL {
	if testDB == nil {
		testDB, _ = gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	}
	dal := database.NewPenguinDAL(testDB)
	_ = dal.DefineTable(&database.AuditLog{})
	// Clean up for each test
	testDB.Exec("DELETE FROM audit_logs")
	return dal
}

func TestNewAuditLogger(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	dal := getTestDAL()

	auditLogger, _ := NewAuditLogger(dal, logger)
	if auditLogger == nil {
		t.Fatal("NewAuditLogger returned nil")
	}
	if auditLogger.dal != dal {
		t.Fatal("dal not set correctly")
	}
}

func TestAppend_ValidEvent(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	dal := getTestDAL()
	auditLogger, _ := NewAuditLogger(dal, logger)

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

	var count int64
	dal.Query().Model(&database.AuditLog{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 event in DB, got %d", count)
	}
}

func TestAppend_GeneratesID(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	dal := getTestDAL()
	auditLogger, _ := NewAuditLogger(dal, logger)

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

	if event.ID == "" || event.ID == "0" {
		t.Fatal("ID was not generated or is zero")
	}
}

func TestAppend_GeneratesTimestamp(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	dal := getTestDAL()
	auditLogger, _ := NewAuditLogger(dal, logger)

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

	if event.Timestamp.Before(before.Add(-1*time.Second)) || event.Timestamp.After(after.Add(1*time.Second)) {
		t.Errorf("Timestamp not in expected range: %v (expected between %v and %v)", event.Timestamp, before, after)
	}
}

func TestAppend_MissingTenant(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	dal := getTestDAL()
	auditLogger, _ := NewAuditLogger(dal, logger)

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
}

func TestQuery_FilterByAction(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	dal := getTestDAL()
	auditLogger, _ := NewAuditLogger(dal, logger)

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

func TestQuery_LimitAndOffset(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	dal := getTestDAL()
	auditLogger, _ := NewAuditLogger(dal, logger)

	// Add 5 events
	for i := 1; i <= 5; i++ {
		auditLogger.Append(&AuditEvent{
			Tenant:   "tenant-1",
			Actor:    fmt.Sprintf("user-%d", i),
			Action:   "create",
			Resource: fmt.Sprintf("resource-%d", i),
			Outcome:  "success",
		})
	}

	// Test limit
	filter := AuditFilter{Limit: 2}
	results := auditLogger.Query(filter)

	if len(results) != 2 {
		t.Errorf("expected 2 results with limit 2, got %d", len(results))
	}

	// Test offset
	filter = AuditFilter{Offset: 3, Limit: 100}
	results = auditLogger.Query(filter)

	if len(results) != 2 {
		t.Errorf("expected 2 results with offset 3, got %d", len(results))
	}
}

func TestAppend_Concurrent(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	dal := getTestDAL()
	auditLogger, _ := NewAuditLogger(dal, logger)

	done := make(chan bool)

	// Simulate concurrent appends
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

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	var count int64
	dal.Query().Model(&database.AuditLog{}).Count(&count)
	if count != 10 {
		t.Errorf("expected 10 events in DB, got %d", count)
	}
}
