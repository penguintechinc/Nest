package main

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// TestDurableStoreInit tests SQLite initialization (fallback when testcontainers unavailable).
func TestDurableStoreInit_SQLite(t *testing.T) {
	// Use a temporary database file for testing
	tmpDir := t.TempDir()
	dbPath := fmt.Sprintf("%s/test-audit.db", tmpDir)

	os.Setenv("DB_TYPE", "sqlite")
	os.Setenv("DB_NAME", dbPath)
	defer os.Unsetenv("DB_TYPE")
	defer os.Unsetenv("DB_NAME")

	logger := getTestLogger()
	defer logger.Sync()

	store, err := NewDurableStore(logger)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	if store == nil {
		t.Fatal("store is nil")
	}
	if store.db == nil {
		t.Fatal("store.db is nil")
	}
}

// TestAppendAndQuery tests basic append and query functionality.
func TestAppendAndQuery_SQLite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := fmt.Sprintf("%s/test-audit.db", tmpDir)

	os.Setenv("DB_TYPE", "sqlite")
	os.Setenv("DB_NAME", dbPath)
	defer os.Unsetenv("DB_TYPE")
	defer os.Unsetenv("DB_NAME")

	logger := getTestLogger()
	defer logger.Sync()

	store, err := NewDurableStore(logger)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Append an event
	event := &AuditEvent{
		Tenant:   "tenant-1",
		Actor:    "user-123", // UUID, not PII
		Action:   "create",
		Resource: "document",
		Outcome:  "success",
		Details: map[string]interface{}{
			"doc_id": "doc-456",
		},
	}

	err = store.Append(event)
	if err != nil {
		t.Fatalf("failed to append event: %v", err)
	}

	if event.ID == "" {
		t.Fatal("event ID not generated")
	}
	if event.Timestamp.IsZero() {
		t.Fatal("event timestamp not set")
	}

	// Query the event
	filter := AuditFilter{Tenant: "tenant-1", Limit: 100}
	results := store.Query(filter)

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}

	if results[0].Tenant != "tenant-1" {
		t.Errorf("expected tenant-1, got %s", results[0].Tenant)
	}
	if results[0].Action != "create" {
		t.Errorf("expected action 'create', got %s", results[0].Action)
	}
}

// TestTenantIsolation tests that queries are properly tenant-scoped.
func TestTenantIsolation_SQLite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := fmt.Sprintf("%s/test-audit.db", tmpDir)

	os.Setenv("DB_TYPE", "sqlite")
	os.Setenv("DB_NAME", dbPath)
	defer os.Unsetenv("DB_TYPE")
	defer os.Unsetenv("DB_NAME")

	logger := getTestLogger()
	defer logger.Sync()

	store, err := NewDurableStore(logger)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Append events for different tenants
	event1 := &AuditEvent{
		Tenant:   "tenant-1",
		Actor:    "user-1",
		Action:   "create",
		Resource: "doc-1",
		Outcome:  "success",
	}
	event2 := &AuditEvent{
		Tenant:   "tenant-2",
		Actor:    "user-2",
		Action:   "delete",
		Resource: "doc-2",
		Outcome:  "success",
	}

	store.Append(event1)
	store.Append(event2)

	// Query for tenant-1
	filter1 := AuditFilter{Tenant: "tenant-1", Limit: 100}
	results1 := store.Query(filter1)

	if len(results1) != 1 {
		t.Errorf("tenant-1 query: expected 1 result, got %d", len(results1))
	}

	// Query for tenant-2
	filter2 := AuditFilter{Tenant: "tenant-2", Limit: 100}
	results2 := store.Query(filter2)

	if len(results2) != 1 {
		t.Errorf("tenant-2 query: expected 1 result, got %d", len(results2))
	}

	// Verify isolation
	if results1[0].Tenant != "tenant-1" || results2[0].Tenant != "tenant-2" {
		t.Fatal("tenant isolation failed")
	}
}

// TestHashChain tests that the hash chain is correctly formed.
func TestHashChain_SQLite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := fmt.Sprintf("%s/test-audit.db", tmpDir)

	os.Setenv("DB_TYPE", "sqlite")
	os.Setenv("DB_NAME", dbPath)
	defer os.Unsetenv("DB_TYPE")
	defer os.Unsetenv("DB_NAME")

	logger := getTestLogger()
	defer logger.Sync()

	store, err := NewDurableStore(logger)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Append N events
	N := 5
	for i := 1; i <= N; i++ {
		event := &AuditEvent{
			Tenant:   "tenant-1",
			Actor:    fmt.Sprintf("user-%d", i),
			Action:   "action",
			Resource: fmt.Sprintf("resource-%d", i),
			Outcome:  "success",
		}
		err := store.Append(event)
		if err != nil {
			t.Fatalf("failed to append event %d: %v", i, err)
		}
	}

	// Fetch all records to verify hash chain
	var records []AuditEventRecord
	store.db.Order("seq ASC").Find(&records)

	if len(records) != N {
		t.Errorf("expected %d records, got %d", N, len(records))
	}

	// Verify each record has a hash
	for i, record := range records {
		if record.Hash == "" {
			t.Errorf("record %d has empty hash", i+1)
		}
		if i > 0 {
			if record.PrevHash == "" {
				t.Errorf("record %d has empty prev_hash", i+1)
			}
			if record.PrevHash != records[i-1].Hash {
				t.Errorf("record %d prev_hash doesn't match previous record's hash", i+1)
			}
		} else if record.PrevHash != "" {
			t.Errorf("first record should have empty prev_hash, got %s", record.PrevHash)
		}
	}
}

// TestValidateIntegrity_CleanChain tests that ValidateIntegrity passes on clean chain.
func TestValidateIntegrity_CleanChain_SQLite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := fmt.Sprintf("%s/test-audit.db", tmpDir)

	os.Setenv("DB_TYPE", "sqlite")
	os.Setenv("DB_NAME", dbPath)
	defer os.Unsetenv("DB_TYPE")
	defer os.Unsetenv("DB_NAME")

	logger := getTestLogger()
	defer logger.Sync()

	store, err := NewDurableStore(logger)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Append events
	for i := 1; i <= 3; i++ {
		event := &AuditEvent{
			Tenant:   "tenant-1",
			Actor:    fmt.Sprintf("user-%d", i),
			Action:   "create",
			Resource: fmt.Sprintf("resource-%d", i),
			Outcome:  "success",
		}
		store.Append(event)
	}

	// Validate integrity
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = store.ValidateIntegrity(ctx)
	if err != nil {
		t.Errorf("expected clean chain to pass validation, got error: %v", err)
	}
}

// TestValidateIntegrity_TamperedChain tests that ValidateIntegrity detects tampering.
func TestValidateIntegrity_TamperedChain_SQLite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := fmt.Sprintf("%s/test-audit.db", tmpDir)

	os.Setenv("DB_TYPE", "sqlite")
	os.Setenv("DB_NAME", dbPath)
	defer os.Unsetenv("DB_TYPE")
	defer os.Unsetenv("DB_NAME")

	logger := getTestLogger()
	defer logger.Sync()

	store, err := NewDurableStore(logger)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Append events
	for i := 1; i <= 3; i++ {
		event := &AuditEvent{
			Tenant:   "tenant-1",
			Actor:    fmt.Sprintf("user-%d", i),
			Action:   "create",
			Resource: fmt.Sprintf("resource-%d", i),
			Outcome:  "success",
		}
		store.Append(event)
	}

	// Simulate tampering: update a record's action field
	// This should cause hash mismatch
	store.db.Model(&AuditEventRecord{}).Where("seq = ?", 2).Update("action", "MODIFIED")

	// Validate integrity - should fail
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = store.ValidateIntegrity(ctx)
	if err == nil {
		t.Error("expected tampering detection, but validation passed")
	} else if err.Error() == "" {
		t.Error("expected error message, got empty string")
	}
}

// TestDurability tests that events persist across reconnection.
func TestDurability_SQLite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := fmt.Sprintf("%s/test-audit.db", tmpDir)

	os.Setenv("DB_TYPE", "sqlite")
	os.Setenv("DB_NAME", dbPath)
	defer os.Unsetenv("DB_TYPE")
	defer os.Unsetenv("DB_NAME")

	logger := getTestLogger()
	defer logger.Sync()

	// Create first store and append events
	store1, err := NewDurableStore(logger)
	if err != nil {
		t.Fatalf("failed to create store1: %v", err)
	}

	event := &AuditEvent{
		Tenant:   "tenant-1",
		Actor:    "user-1",
		Action:   "create",
		Resource: "doc-1",
		Outcome:  "success",
	}
	store1.Append(event)
	store1.Close()

	// Create second store (new connection to same DB)
	store2, err := NewDurableStore(logger)
	if err != nil {
		t.Fatalf("failed to create store2: %v", err)
	}
	defer store2.Close()

	// Query - should find the appended event
	filter := AuditFilter{Tenant: "tenant-1", Limit: 100}
	results := store2.Query(filter)

	if len(results) != 1 {
		t.Errorf("expected 1 result after reconnection, got %d", len(results))
	}

	if results[0].Action != "create" {
		t.Errorf("expected action 'create', got %s", results[0].Action)
	}
}

// TestConcurrentAppends tests that concurrent appends maintain chain integrity.
func TestConcurrentAppends_SQLite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := fmt.Sprintf("%s/test-audit.db", tmpDir)

	os.Setenv("DB_TYPE", "sqlite")
	os.Setenv("DB_NAME", dbPath)
	defer os.Unsetenv("DB_TYPE")
	defer os.Unsetenv("DB_NAME")

	logger := getTestLogger()
	defer logger.Sync()

	store, err := NewDurableStore(logger)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Append events concurrently
	numEvents := 10
	done := make(chan error, numEvents)

	for i := 1; i <= numEvents; i++ {
		go func(id int) {
			event := &AuditEvent{
				Tenant:   "tenant-1",
				Actor:    fmt.Sprintf("user-%d", id),
				Action:   "create",
				Resource: fmt.Sprintf("resource-%d", id),
				Outcome:  "success",
			}
			err := store.Append(event)
			done <- err
		}(i)
	}

	// Wait for all appends to complete
	var appendErrors []error
	for i := 0; i < numEvents; i++ {
		if err := <-done; err != nil {
			appendErrors = append(appendErrors, err)
		}
	}

	if len(appendErrors) > 0 {
		t.Errorf("got %d append errors", len(appendErrors))
		for _, e := range appendErrors {
			t.Logf("  %v", e)
		}
	}

	// Verify all events were appended
	filter := AuditFilter{Tenant: "tenant-1", Limit: 1000}
	results := store.Query(filter)

	if len(results) != numEvents {
		t.Errorf("expected %d events, got %d", numEvents, len(results))
	}

	// Validate integrity - chain should be intact despite concurrent appends
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = store.ValidateIntegrity(ctx)
	if err != nil {
		t.Errorf("integrity validation failed after concurrent appends: %v", err)
	}
}

// TestFilterByAction tests action filtering.
func TestFilterByAction_SQLite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := fmt.Sprintf("%s/test-audit.db", tmpDir)

	os.Setenv("DB_TYPE", "sqlite")
	os.Setenv("DB_NAME", dbPath)
	defer os.Unsetenv("DB_TYPE")
	defer os.Unsetenv("DB_NAME")

	logger := getTestLogger()
	defer logger.Sync()

	store, err := NewDurableStore(logger)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Append events with different actions
	store.Append(&AuditEvent{
		Tenant:   "tenant-1",
		Actor:    "user-1",
		Action:   "create",
		Resource: "doc",
		Outcome:  "success",
	})
	store.Append(&AuditEvent{
		Tenant:   "tenant-1",
		Actor:    "user-1",
		Action:   "delete",
		Resource: "doc",
		Outcome:  "success",
	})

	// Query by action
	filter := AuditFilter{Tenant: "tenant-1", Action: "create", Limit: 100}
	results := store.Query(filter)

	if len(results) != 1 || results[0].Action != "create" {
		t.Error("action filter failed")
	}
}

// TestTimeRangeFilter tests time range filtering.
func TestTimeRangeFilter_SQLite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := fmt.Sprintf("%s/test-audit.db", tmpDir)

	os.Setenv("DB_TYPE", "sqlite")
	os.Setenv("DB_NAME", dbPath)
	defer os.Unsetenv("DB_TYPE")
	defer os.Unsetenv("DB_NAME")

	logger := getTestLogger()
	defer logger.Sync()

	store, err := NewDurableStore(logger)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	now := time.Now()
	pastTime := now.Add(-1 * time.Hour)
	futureTime := now.Add(1 * time.Hour)

	// Append event with past timestamp
	store.Append(&AuditEvent{
		Tenant:    "tenant-1",
		Actor:     "user-1",
		Action:    "create",
		Resource:  "doc-1",
		Outcome:   "success",
		Timestamp: pastTime,
	})

	// Append event with future timestamp
	store.Append(&AuditEvent{
		Tenant:    "tenant-1",
		Actor:     "user-1",
		Action:    "create",
		Resource:  "doc-2",
		Outcome:   "success",
		Timestamp: futureTime,
	})

	// Query with time range
	filter := AuditFilter{Tenant: "tenant-1", StartTime: now, Limit: 100}
	results := store.Query(filter)

	if len(results) != 1 {
		t.Errorf("expected 1 result in time range, got %d", len(results))
	}
}

// TestValidation tests that required field validation works.
func TestValidation_SQLite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := fmt.Sprintf("%s/test-audit.db", tmpDir)

	os.Setenv("DB_TYPE", "sqlite")
	os.Setenv("DB_NAME", dbPath)
	defer os.Unsetenv("DB_TYPE")
	defer os.Unsetenv("DB_NAME")

	logger := getTestLogger()
	defer logger.Sync()

	store, err := NewDurableStore(logger)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Test missing required fields
	testCases := []struct {
		name   string
		event  *AuditEvent
		errMsg string
	}{
		{
			name: "missing tenant",
			event: &AuditEvent{
				Actor:    "user-1",
				Action:   "create",
				Resource: "doc",
				Outcome:  "success",
			},
			errMsg: "tenant is required",
		},
		{
			name: "missing actor",
			event: &AuditEvent{
				Tenant:   "tenant-1",
				Action:   "create",
				Resource: "doc",
				Outcome:  "success",
			},
			errMsg: "actor is required",
		},
		{
			name: "missing action",
			event: &AuditEvent{
				Tenant:   "tenant-1",
				Actor:    "user-1",
				Resource: "doc",
				Outcome:  "success",
			},
			errMsg: "action is required",
		},
		{
			name: "missing resource",
			event: &AuditEvent{
				Tenant:  "tenant-1",
				Actor:   "user-1",
				Action:  "create",
				Outcome: "success",
			},
			errMsg: "resource is required",
		},
		{
			name: "missing outcome",
			event: &AuditEvent{
				Tenant:   "tenant-1",
				Actor:    "user-1",
				Action:   "create",
				Resource: "doc",
			},
			errMsg: "outcome is required",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := store.Append(tc.event)
			if err == nil {
				t.Errorf("expected error for %s", tc.name)
			} else if err.Error() != tc.errMsg {
				t.Errorf("expected '%s', got '%s'", tc.errMsg, err.Error())
			}
		})
	}
}
