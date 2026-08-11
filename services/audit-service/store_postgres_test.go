package main

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestPostgresIntegration tests the durable store against a real PostgreSQL database.
// This test requires Docker to be available.
func TestPostgresIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Start PostgreSQL container
	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-bookworm",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "postgres",
			"POSTGRES_PASSWORD": "password",
			"POSTGRES_DB":       "audit",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).
			WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Skipf("PostgreSQL testcontainer failed to start (Docker may not be available): %v", err)
	}
	defer container.Terminate(ctx)

	// Get connection details
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get container host: %v", err)
	}

	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("failed to get mapped port: %v", err)
	}

	// Set environment for GORM
	os.Setenv("DB_TYPE", "postgresql")
	os.Setenv("DB_HOST", host)
	os.Setenv("DB_PORT", port.Port())
	os.Setenv("DB_NAME", "audit")
	os.Setenv("DB_USER", "postgres")
	os.Setenv("DB_PASS", "password")
	defer func() {
		os.Unsetenv("DB_TYPE")
		os.Unsetenv("DB_HOST")
		os.Unsetenv("DB_PORT")
		os.Unsetenv("DB_NAME")
		os.Unsetenv("DB_USER")
		os.Unsetenv("DB_PASS")
	}()

	logger := getTestLogger()
	defer logger.Sync()

	store, err := NewDurableStore(logger)
	if err != nil {
		t.Fatalf("failed to create durable store: %v", err)
	}
	defer store.Close()

	// Run integration tests
	t.Run("Postgres append and query", func(t *testing.T) {
		event := &AuditEvent{
			Tenant:   "tenant-1",
			Actor:    "user-123",
			Action:   "create",
			Resource: "document",
			Outcome:  "success",
			SourceIP: "192.168.1.1",
			Details: map[string]interface{}{
				"doc_id": "doc-456",
			},
		}

		err := store.Append(event)
		if err != nil {
			t.Fatalf("failed to append event: %v", err)
		}

		filter := AuditFilter{Tenant: "tenant-1", Limit: 100}
		results := store.Query(filter)

		if len(results) != 1 {
			t.Errorf("expected 1 result, got %d", len(results))
		}

		if results[0].Outcome != "success" {
			t.Errorf("outcome not preserved: got %s", results[0].Outcome)
		}
		if results[0].SourceIP != "192.168.1.1" {
			t.Errorf("source_ip not preserved: got %s", results[0].SourceIP)
		}
	})

	t.Run("Postgres tenant isolation", func(t *testing.T) {
		store.db.Exec("DELETE FROM audit_events") // Clean slate

		store.Append(&AuditEvent{
			Tenant:   "tenant-1",
			Actor:    "user-1",
			Action:   "create",
			Resource: "doc-1",
			Outcome:  "success",
		})
		store.Append(&AuditEvent{
			Tenant:   "tenant-2",
			Actor:    "user-2",
			Action:   "delete",
			Resource: "doc-2",
			Outcome:  "success",
		})

		filter1 := AuditFilter{Tenant: "tenant-1", Limit: 100}
		results1 := store.Query(filter1)
		if len(results1) != 1 || results1[0].Tenant != "tenant-1" {
			t.Error("tenant-1 isolation failed")
		}

		filter2 := AuditFilter{Tenant: "tenant-2", Limit: 100}
		results2 := store.Query(filter2)
		if len(results2) != 1 || results2[0].Tenant != "tenant-2" {
			t.Error("tenant-2 isolation failed")
		}
	})

	t.Run("Postgres hash chain", func(t *testing.T) {
		store.db.Exec("DELETE FROM audit_events")

		for i := 1; i <= 3; i++ {
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

		var records []AuditEventRecord
		store.db.Order("seq ASC").Find(&records)

		if len(records) != 3 {
			t.Errorf("expected 3 records, got %d", len(records))
		}

		for i, record := range records {
			if record.Hash == "" {
				t.Errorf("record %d has empty hash", i+1)
			}
			if i > 0 && record.PrevHash != records[i-1].Hash {
				t.Errorf("record %d prev_hash doesn't match previous record's hash", i+1)
			}
		}
	})

	t.Run("Postgres tamper detection", func(t *testing.T) {
		store.db.Exec("DELETE FROM audit_events")

		store.Append(&AuditEvent{
			Tenant:   "tenant-1",
			Actor:    "user-1",
			Action:   "action",
			Resource: "resource",
			Outcome:  "success",
		})

		// Simulate tampering
		store.db.Model(&AuditEventRecord{}).Where("seq = ?", 1).Update("outcome", "denied")

		err := store.ValidateIntegrity(context.Background())
		if err == nil {
			t.Error("expected tampering detection, but validation passed")
		}
		if fmt.Sprintf("%v", err) == "" {
			t.Error("expected error message, got empty string")
		}
	})

	t.Run("Postgres outcome filtering", func(t *testing.T) {
		store.db.Exec("DELETE FROM audit_events")

		store.Append(&AuditEvent{
			Tenant:   "tenant-1",
			Actor:    "user-1",
			Action:   "read",
			Resource: "doc",
			Outcome:  "success",
		})
		store.Append(&AuditEvent{
			Tenant:   "tenant-1",
			Actor:    "user-1",
			Action:   "write",
			Resource: "doc",
			Outcome:  "denied",
		})

		filter := AuditFilter{Tenant: "tenant-1", Outcome: "denied", Limit: 100}
		results := store.Query(filter)

		if len(results) != 1 {
			t.Errorf("expected 1 denied result, got %d", len(results))
		}
		if results[0].Outcome != "denied" {
			t.Errorf("expected denied, got %s", results[0].Outcome)
		}
	})
}
