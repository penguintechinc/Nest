package main

import (
	"testing"
	"time"
)

func TestNewCatalog(t *testing.T) {
	catalog := NewCatalog()
	if catalog == nil {
		t.Errorf("expected non-nil catalog")
	}
	if catalog.entries == nil {
		t.Errorf("expected initialized entries map")
	}
}

func TestCatalogUpsertNew(t *testing.T) {
	catalog := NewCatalog()
	entry := &CatalogEntry{
		Tenant:      "tenant-1",
		ResourceID:  "resource-1",
		BackendType: "postgres",
		TableName:   "users",
		DiscoveredAt: time.Now(),
		UpdatedAt:   time.Now(),
	}

	catalog.Upsert(entry)

	if entry.ID == "" {
		t.Errorf("expected ID to be assigned")
	}
	if entry.LabelConfidence == nil {
		t.Errorf("expected LabelConfidence to be initialized")
	}

	stored, ok := catalog.Get(entry.ID)
	if !ok {
		t.Errorf("expected to retrieve stored entry")
	}
	if stored.ResourceID != "resource-1" {
		t.Errorf("expected stored entry to match original")
	}
}

func TestCatalogUpsertWithExistingID(t *testing.T) {
	catalog := NewCatalog()
	entry := &CatalogEntry{
		ID:          "custom-id-123",
		Tenant:      "tenant-1",
		ResourceID:  "resource-1",
		BackendType: "postgres",
		TableName:   "users",
		DiscoveredAt: time.Now(),
		UpdatedAt:   time.Now(),
	}

	catalog.Upsert(entry)
	if entry.ID != "custom-id-123" {
		t.Errorf("expected ID to remain unchanged")
	}
}

func TestCatalogUpsertUpdate(t *testing.T) {
	catalog := NewCatalog()

	// First upsert
	entry1 := &CatalogEntry{
		Tenant:      "tenant-1",
		ResourceID:  "resource-1",
		BackendType: "postgres",
		TableName:   "users",
		Labels:      []string{"PII"},
		DiscoveredAt: time.Now(),
		UpdatedAt:   time.Now(),
	}
	catalog.Upsert(entry1)

	// Second upsert with same resource/table (should update, not create)
	entry2 := &CatalogEntry{
		Tenant:      "tenant-1",
		ResourceID:  "resource-1",
		BackendType: "postgres",
		TableName:   "users",
		Labels:      []string{"PII", "PCI"},
		DiscoveredAt: time.Now(),
		UpdatedAt:   time.Now(),
	}
	catalog.Upsert(entry2)

	// Both should map to the same key
	stats := catalog.Stats()
	if stats["total_entries"] != 1 {
		t.Errorf("expected 1 entry after update, got %d", stats["total_entries"])
	}
}

func TestCatalogGetByID(t *testing.T) {
	catalog := NewCatalog()
	entry := &CatalogEntry{
		Tenant:      "tenant-1",
		ResourceID:  "resource-1",
		BackendType: "postgres",
		TableName:   "users",
		DiscoveredAt: time.Now(),
		UpdatedAt:   time.Now(),
	}
	catalog.Upsert(entry)

	// Get existing entry
	retrieved, ok := catalog.Get(entry.ID)
	if !ok {
		t.Errorf("expected to find entry by ID")
	}
	if retrieved.Tenant != "tenant-1" {
		t.Errorf("expected tenant to match")
	}

	// Get non-existent entry
	_, ok = catalog.Get("non-existent-id")
	if ok {
		t.Errorf("expected not to find non-existent entry")
	}
}

func TestCatalogListNoFilter(t *testing.T) {
	catalog := NewCatalog()

	for i := 1; i <= 3; i++ {
		entry := &CatalogEntry{
			Tenant:      "tenant-" + string(rune(48+i)),
			ResourceID:  "resource-" + string(rune(48+i)),
			BackendType: "postgres",
			TableName:   "table-" + string(rune(48+i)),
			DiscoveredAt: time.Now(),
			UpdatedAt:   time.Now(),
		}
		catalog.Upsert(entry)
	}

	// List all (no filters)
	results := catalog.List("", "")
	if len(results) != 3 {
		t.Errorf("expected 3 entries when no filters, got %d", len(results))
	}
}

func TestCatalogListFilterByTenant(t *testing.T) {
	catalog := NewCatalog()

	entry1 := &CatalogEntry{
		Tenant:      "tenant-1",
		ResourceID:  "resource-1",
		BackendType: "postgres",
		TableName:   "users",
		DiscoveredAt: time.Now(),
		UpdatedAt:   time.Now(),
	}
	catalog.Upsert(entry1)

	entry2 := &CatalogEntry{
		Tenant:      "tenant-2",
		ResourceID:  "resource-2",
		BackendType: "postgres",
		TableName:   "accounts",
		DiscoveredAt: time.Now(),
		UpdatedAt:   time.Now(),
	}
	catalog.Upsert(entry2)

	entry3 := &CatalogEntry{
		Tenant:      "tenant-1",
		ResourceID:  "resource-3",
		BackendType: "mysql",
		TableName:   "logs",
		DiscoveredAt: time.Now(),
		UpdatedAt:   time.Now(),
	}
	catalog.Upsert(entry3)

	// Filter by tenant-1
	results := catalog.List("tenant-1", "")
	if len(results) != 2 {
		t.Errorf("expected 2 entries for tenant-1, got %d", len(results))
	}

	// Filter by tenant-2
	results = catalog.List("tenant-2", "")
	if len(results) != 1 {
		t.Errorf("expected 1 entry for tenant-2, got %d", len(results))
	}
}

func TestCatalogListFilterByBackendType(t *testing.T) {
	catalog := NewCatalog()

	entry1 := &CatalogEntry{
		Tenant:      "tenant-1",
		ResourceID:  "resource-1",
		BackendType: "postgres",
		TableName:   "users",
		DiscoveredAt: time.Now(),
		UpdatedAt:   time.Now(),
	}
	catalog.Upsert(entry1)

	entry2 := &CatalogEntry{
		Tenant:      "tenant-1",
		ResourceID:  "resource-2",
		BackendType: "mysql",
		TableName:   "accounts",
		DiscoveredAt: time.Now(),
		UpdatedAt:   time.Now(),
	}
	catalog.Upsert(entry2)

	entry3 := &CatalogEntry{
		Tenant:      "tenant-2",
		ResourceID:  "resource-3",
		BackendType: "postgres",
		TableName:   "logs",
		DiscoveredAt: time.Now(),
		UpdatedAt:   time.Now(),
	}
	catalog.Upsert(entry3)

	// Filter by postgres
	results := catalog.List("", "postgres")
	if len(results) != 2 {
		t.Errorf("expected 2 postgres entries, got %d", len(results))
	}

	// Filter by mysql
	results = catalog.List("", "mysql")
	if len(results) != 1 {
		t.Errorf("expected 1 mysql entry, got %d", len(results))
	}
}

func TestCatalogListFilterByBoth(t *testing.T) {
	catalog := NewCatalog()

	entry1 := &CatalogEntry{
		Tenant:      "tenant-1",
		ResourceID:  "resource-1",
		BackendType: "postgres",
		TableName:   "users",
		DiscoveredAt: time.Now(),
		UpdatedAt:   time.Now(),
	}
	catalog.Upsert(entry1)

	entry2 := &CatalogEntry{
		Tenant:      "tenant-1",
		ResourceID:  "resource-2",
		BackendType: "mysql",
		TableName:   "accounts",
		DiscoveredAt: time.Now(),
		UpdatedAt:   time.Now(),
	}
	catalog.Upsert(entry2)

	entry3 := &CatalogEntry{
		Tenant:      "tenant-2",
		ResourceID:  "resource-3",
		BackendType: "postgres",
		TableName:   "logs",
		DiscoveredAt: time.Now(),
		UpdatedAt:   time.Now(),
	}
	catalog.Upsert(entry3)

	// Filter by tenant-1 AND postgres
	results := catalog.List("tenant-1", "postgres")
	if len(results) != 1 {
		t.Errorf("expected 1 entry for tenant-1 + postgres, got %d", len(results))
	}
	if results[0].ResourceID != "resource-1" {
		t.Errorf("expected resource-1, got %s", results[0].ResourceID)
	}

	// Filter by tenant-2 AND postgres
	results = catalog.List("tenant-2", "postgres")
	if len(results) != 1 {
		t.Errorf("expected 1 entry for tenant-2 + postgres, got %d", len(results))
	}
	if results[0].ResourceID != "resource-3" {
		t.Errorf("expected resource-3, got %s", results[0].ResourceID)
	}

	// Filter by tenant-1 AND mysql
	results = catalog.List("tenant-1", "mysql")
	if len(results) != 1 {
		t.Errorf("expected 1 entry for tenant-1 + mysql, got %d", len(results))
	}
	if results[0].ResourceID != "resource-2" {
		t.Errorf("expected resource-2, got %s", results[0].ResourceID)
	}
}

func TestCatalogApplyLabels(t *testing.T) {
	catalog := NewCatalog()
	entry := &CatalogEntry{
		Tenant:      "tenant-1",
		ResourceID:  "resource-1",
		BackendType: "postgres",
		TableName:   "users",
		DiscoveredAt: time.Now(),
		UpdatedAt:   time.Now(),
	}
	catalog.Upsert(entry)

	labels := map[string]float64{
		"PII":  92.0,
		"PHI":  85.0,
	}

	err := catalog.ApplyLabels("resource-1", "users", labels)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Verify labels were applied
	retrieved, _ := catalog.Get(entry.ID)
	if len(retrieved.Labels) != 2 {
		t.Errorf("expected 2 labels, got %d", len(retrieved.Labels))
	}
	if len(retrieved.LabelConfidence) != 2 {
		t.Errorf("expected 2 label confidences, got %d", len(retrieved.LabelConfidence))
	}
	if retrieved.LabelConfidence["PII"] != 92.0 {
		t.Errorf("expected PII confidence 92.0, got %f", retrieved.LabelConfidence["PII"])
	}
}

func TestCatalogApplyLabelsNotFound(t *testing.T) {
	catalog := NewCatalog()

	labels := map[string]float64{
		"PII": 92.0,
	}

	err := catalog.ApplyLabels("non-existent", "table", labels)
	if err == nil {
		t.Errorf("expected error for non-existent entry")
	}
}

func TestCatalogApplyLabelsUpdateTime(t *testing.T) {
	catalog := NewCatalog()
	originalTime := time.Now().Add(-time.Hour)
	entry := &CatalogEntry{
		Tenant:       "tenant-1",
		ResourceID:   "resource-1",
		BackendType:  "postgres",
		TableName:    "users",
		DiscoveredAt: originalTime,
		UpdatedAt:    originalTime,
	}
	catalog.Upsert(entry)

	// Apply labels
	labels := map[string]float64{"PII": 92.0}
	catalog.ApplyLabels("resource-1", "users", labels)

	// Verify UpdatedAt was updated
	retrieved, _ := catalog.Get(entry.ID)
	if retrieved.UpdatedAt == originalTime {
		t.Errorf("expected UpdatedAt to be updated")
	}
}

func TestCatalogStats(t *testing.T) {
	catalog := NewCatalog()

	// Empty catalog
	stats := catalog.Stats()
	if stats["total_entries"] != 0 {
		t.Errorf("expected 0 total_entries, got %d", stats["total_entries"])
	}
	if stats["total_labels"] != 0 {
		t.Errorf("expected 0 total_labels, got %d", stats["total_labels"])
	}
	if stats["pending_classification"] != 0 {
		t.Errorf("expected 0 pending_classification, got %d", stats["pending_classification"])
	}

	// Add entry without labels
	entry1 := &CatalogEntry{
		Tenant:       "tenant-1",
		ResourceID:   "resource-1",
		BackendType:  "postgres",
		TableName:    "users",
		DiscoveredAt: time.Now(),
		UpdatedAt:    time.Now(),
	}
	catalog.Upsert(entry1)

	stats = catalog.Stats()
	if stats["total_entries"] != 1 {
		t.Errorf("expected 1 total_entries, got %d", stats["total_entries"])
	}
	if stats["pending_classification"] != 1 {
		t.Errorf("expected 1 pending_classification, got %d", stats["pending_classification"])
	}

	// Add entry with labels
	entry2 := &CatalogEntry{
		Tenant:       "tenant-1",
		ResourceID:   "resource-2",
		BackendType:  "mysql",
		TableName:    "accounts",
		Labels:       []string{"PII"},
		DiscoveredAt: time.Now(),
		UpdatedAt:    time.Now(),
		LabelConfidence: map[string]float64{
			"PII": 92.0,
		},
	}
	catalog.Upsert(entry2)

	stats = catalog.Stats()
	if stats["total_entries"] != 2 {
		t.Errorf("expected 2 total_entries, got %d", stats["total_entries"])
	}
	if stats["total_labels"] != 1 {
		t.Errorf("expected 1 total_labels, got %d", stats["total_labels"])
	}
	if stats["pending_classification"] != 1 {
		t.Errorf("expected 1 pending_classification, got %d", stats["pending_classification"])
	}

	// Add entry with multiple labels
	entry3 := &CatalogEntry{
		Tenant:       "tenant-1",
		ResourceID:   "resource-3",
		BackendType:  "postgres",
		TableName:    "logs",
		Labels:       []string{"PII", "PHI", "SENSITIVE"},
		DiscoveredAt: time.Now(),
		UpdatedAt:    time.Now(),
		LabelConfidence: map[string]float64{
			"PII":       92.0,
			"PHI":       90.0,
			"SENSITIVE": 80.0,
		},
	}
	catalog.Upsert(entry3)

	stats = catalog.Stats()
	if stats["total_entries"] != 3 {
		t.Errorf("expected 3 total_entries, got %d", stats["total_entries"])
	}
	if stats["total_labels"] != 4 {
		t.Errorf("expected 4 total_labels, got %d", stats["total_labels"])
	}
	if stats["pending_classification"] != 1 {
		t.Errorf("expected 1 pending_classification, got %d", stats["pending_classification"])
	}
}

func TestCatalogConcurrentOperations(t *testing.T) {
	catalog := NewCatalog()
	done := make(chan bool)

	// Concurrent upserts
	for i := 0; i < 10; i++ {
		go func(idx int) {
			entry := &CatalogEntry{
				Tenant:       "tenant-" + string(rune(48+idx)),
				ResourceID:   "resource-" + string(rune(48+idx)),
				BackendType:  "postgres",
				TableName:    "table-" + string(rune(48+idx)),
				DiscoveredAt: time.Now(),
				UpdatedAt:    time.Now(),
			}
			catalog.Upsert(entry)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	stats := catalog.Stats()
	if stats["total_entries"] != 10 {
		t.Errorf("expected 10 entries after concurrent upserts, got %d", stats["total_entries"])
	}
}

func TestCatalogLabelConfidenceInitialized(t *testing.T) {
	catalog := NewCatalog()
	entry := &CatalogEntry{
		Tenant:       "tenant-1",
		ResourceID:   "resource-1",
		BackendType:  "postgres",
		TableName:    "users",
		DiscoveredAt: time.Now(),
		UpdatedAt:    time.Now(),
	}

	if entry.LabelConfidence != nil {
		t.Errorf("expected nil LabelConfidence before upsert")
	}

	catalog.Upsert(entry)

	if entry.LabelConfidence == nil {
		t.Errorf("expected LabelConfidence to be initialized after upsert")
	}
}

func TestCatalogListOrder(t *testing.T) {
	catalog := NewCatalog()

	// Add multiple entries
	for i := 1; i <= 5; i++ {
		entry := &CatalogEntry{
			Tenant:       "tenant-1",
			ResourceID:   "resource-" + string(rune(47+i)),
			BackendType:  "postgres",
			TableName:    "table-" + string(rune(47+i)),
			DiscoveredAt: time.Now(),
			UpdatedAt:    time.Now(),
		}
		catalog.Upsert(entry)
	}

	results := catalog.List("tenant-1", "postgres")
	if len(results) != 5 {
		t.Errorf("expected 5 results, got %d", len(results))
	}
}

func TestCatalogEmptyFilter(t *testing.T) {
	catalog := NewCatalog()

	entry1 := &CatalogEntry{
		Tenant:       "tenant-1",
		ResourceID:   "resource-1",
		BackendType:  "postgres",
		TableName:    "users",
		DiscoveredAt: time.Now(),
		UpdatedAt:    time.Now(),
	}
	catalog.Upsert(entry1)

	entry2 := &CatalogEntry{
		Tenant:       "",
		ResourceID:   "resource-2",
		BackendType:  "mysql",
		TableName:    "accounts",
		DiscoveredAt: time.Now(),
		UpdatedAt:    time.Now(),
	}
	catalog.Upsert(entry2)

	// Empty tenant filter should match all
	results := catalog.List("", "")
	if len(results) != 2 {
		t.Errorf("expected 2 results with empty filters, got %d", len(results))
	}
}
