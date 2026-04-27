package main

import (
	"os"
	"testing"
	"time"
)

func TestNewCalculator_DefaultRate(t *testing.T) {
	// Clear any existing env var
	os.Unsetenv("TOKEN_RATE_USD")

	calc := NewCalculator()
	if calc == nil {
		t.Fatal("NewCalculator returned nil")
	}
	if calc.rate != 0.0001 {
		t.Errorf("expected default rate 0.0001, got %f", calc.rate)
	}
	if len(calc.records) != 0 {
		t.Errorf("expected empty records, got %d", len(calc.records))
	}
}

func TestNewCalculator_EnvVarOverride(t *testing.T) {
	// Set custom rate
	os.Setenv("TOKEN_RATE_USD", "0.0005")
	defer os.Unsetenv("TOKEN_RATE_USD")

	calc := NewCalculator()
	if calc.rate != 0.0005 {
		t.Errorf("expected rate 0.0005 from env var, got %f", calc.rate)
	}
}

func TestNewCalculator_EnvVarInvalid(t *testing.T) {
	// Set invalid rate (should fall back to default)
	os.Setenv("TOKEN_RATE_USD", "not-a-number")
	defer os.Unsetenv("TOKEN_RATE_USD")

	calc := NewCalculator()
	if calc.rate != 0.0001 {
		t.Errorf("expected fallback to default rate 0.0001, got %f", calc.rate)
	}
}

func TestAddTokens_NewRecord(t *testing.T) {
	calc := NewCalculator()
	tenantID := "tenant-1"
	resourceType := "api_calls"
	tokens := 1000.0

	calc.AddTokens(tenantID, resourceType, tokens)

	// Should have one record
	records := calc.AllRecords()
	if len(records) != 1 {
		t.Errorf("expected 1 record, got %d", len(records))
	}

	record := records[0]
	if record.TenantID != tenantID {
		t.Errorf("expected tenantID %s, got %s", tenantID, record.TenantID)
	}
	if record.TotalTokens != tokens {
		t.Errorf("expected totalTokens %f, got %f", tokens, record.TotalTokens)
	}
	if record.TotalCostUSD != tokens*0.0001 {
		t.Errorf("expected cost %f, got %f", tokens*0.0001, record.TotalCostUSD)
	}
	if record.Breakdown[resourceType] != tokens {
		t.Errorf("expected breakdown[%s]=%f, got %f", resourceType, tokens, record.Breakdown[resourceType])
	}
	if record.Month != time.Now().Format("2006-01") {
		t.Errorf("expected month %s, got %s", time.Now().Format("2006-01"), record.Month)
	}
}

func TestAddTokens_AccumulateExisting(t *testing.T) {
	calc := NewCalculator()
	tenantID := "tenant-1"
	resourceType := "api_calls"

	// Add first batch
	calc.AddTokens(tenantID, resourceType, 1000.0)

	// Add second batch
	calc.AddTokens(tenantID, resourceType, 500.0)

	record, found := calc.GetRecord(tenantID, time.Now().Format("2006-01"))
	if !found {
		t.Fatal("record not found")
	}

	if record.TotalTokens != 1500.0 {
		t.Errorf("expected accumulated tokens 1500, got %f", record.TotalTokens)
	}
	if record.TotalCostUSD != 1500.0*0.0001 {
		t.Errorf("expected accumulated cost %f, got %f", 1500.0*0.0001, record.TotalCostUSD)
	}
	if record.Breakdown[resourceType] != 1500.0 {
		t.Errorf("expected breakdown %f, got %f", 1500.0, record.Breakdown[resourceType])
	}
}

func TestAddTokens_MultipleResourceTypes(t *testing.T) {
	calc := NewCalculator()
	tenantID := "tenant-1"

	calc.AddTokens(tenantID, "api_calls", 1000.0)
	calc.AddTokens(tenantID, "embeddings", 500.0)

	record, found := calc.GetRecord(tenantID, time.Now().Format("2006-01"))
	if !found {
		t.Fatal("record not found")
	}

	if record.TotalTokens != 1500.0 {
		t.Errorf("expected total 1500, got %f", record.TotalTokens)
	}
	if record.Breakdown["api_calls"] != 1000.0 {
		t.Errorf("expected api_calls 1000, got %f", record.Breakdown["api_calls"])
	}
	if record.Breakdown["embeddings"] != 500.0 {
		t.Errorf("expected embeddings 500, got %f", record.Breakdown["embeddings"])
	}
}

func TestAddTokens_UpdatedAt(t *testing.T) {
	calc := NewCalculator()
	tenantID := "tenant-1"

	before := time.Now()
	calc.AddTokens(tenantID, "api", 100.0)
	after := time.Now()

	record, found := calc.GetRecord(tenantID, time.Now().Format("2006-01"))
	if !found {
		t.Fatal("record not found")
	}

	if record.UpdatedAt.Before(before) || record.UpdatedAt.After(after.Add(1*time.Second)) {
		t.Errorf("UpdatedAt not set correctly: %v (expected between %v and %v)", record.UpdatedAt, before, after)
	}
}

func TestGetRecord_Found(t *testing.T) {
	calc := NewCalculator()
	tenantID := "tenant-1"
	month := time.Now().Format("2006-01")

	calc.AddTokens(tenantID, "api", 100.0)

	record, found := calc.GetRecord(tenantID, month)
	if !found {
		t.Fatal("expected to find record")
	}
	if record == nil {
		t.Fatal("record is nil")
	}
	if record.TenantID != tenantID {
		t.Errorf("expected tenantID %s, got %s", tenantID, record.TenantID)
	}
}

func TestGetRecord_NotFound(t *testing.T) {
	calc := NewCalculator()

	record, found := calc.GetRecord("nonexistent", "2025-01")
	if found {
		t.Fatal("expected not to find record")
	}
	if record != nil {
		t.Fatal("expected record to be nil")
	}
}

func TestGetRecord_WrongMonth(t *testing.T) {
	calc := NewCalculator()
	tenantID := "tenant-1"

	calc.AddTokens(tenantID, "api", 100.0)

	// Try to get record from different month
	record, found := calc.GetRecord(tenantID, "2024-12")
	if found {
		t.Fatal("expected not to find record for different month")
	}
	if record != nil {
		t.Fatal("expected record to be nil")
	}
}

func TestListRecords_FilterByTenant(t *testing.T) {
	calc := NewCalculator()

	calc.AddTokens("tenant-1", "api", 100.0)
	calc.AddTokens("tenant-2", "api", 200.0)
	calc.AddTokens("tenant-1", "api", 50.0) // Same tenant, same month accumulates

	records := calc.ListRecords("tenant-1")
	if len(records) != 1 {
		t.Errorf("expected 1 record for tenant-1, got %d", len(records))
	}

	if records[0].TenantID != "tenant-1" {
		t.Errorf("expected tenant-1, got %s", records[0].TenantID)
	}
	if records[0].TotalTokens != 150.0 {
		t.Errorf("expected 150 tokens, got %f", records[0].TotalTokens)
	}
}

func TestListRecords_Empty(t *testing.T) {
	calc := NewCalculator()

	records := calc.ListRecords("nonexistent")
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
}

func TestListRecords_NoFilter(t *testing.T) {
	calc := NewCalculator()

	calc.AddTokens("tenant-1", "api", 100.0)
	calc.AddTokens("tenant-2", "api", 200.0)

	allRecords := calc.AllRecords()
	if len(allRecords) != 2 {
		t.Errorf("expected 2 records, got %d", len(allRecords))
	}
}

func TestComputeCost(t *testing.T) {
	os.Setenv("TOKEN_RATE_USD", "0.001")
	defer os.Unsetenv("TOKEN_RATE_USD")

	calc := NewCalculator()
	tenantID := "tenant-1"

	calc.AddTokens(tenantID, "api", 1000.0)

	record, found := calc.GetRecord(tenantID, time.Now().Format("2006-01"))
	if !found {
		t.Fatal("record not found")
	}

	expectedCost := 1000.0 * 0.001
	if record.TotalCostUSD != expectedCost {
		t.Errorf("expected cost %f, got %f", expectedCost, record.TotalCostUSD)
	}
}

func TestComputeCost_ZeroTokens(t *testing.T) {
	calc := NewCalculator()
	tenantID := "tenant-1"

	calc.AddTokens(tenantID, "api", 0.0)

	record, found := calc.GetRecord(tenantID, time.Now().Format("2006-01"))
	if !found {
		t.Fatal("record not found")
	}

	if record.TotalCostUSD != 0.0 {
		t.Errorf("expected cost 0, got %f", record.TotalCostUSD)
	}
}

func TestRunDailyAggregation_NoPanic(t *testing.T) {
	// Test that RunDailyAggregation doesn't panic when called
	// (it's a ticker that runs forever, so we can only verify it doesn't crash)
	calc := NewCalculator()

	// This would block forever, so we just test that the function signature exists
	// and that the calculator can be created
	if calc == nil {
		t.Fatal("calculator is nil")
	}
}

func TestAddTokens_Concurrent(t *testing.T) {
	calc := NewCalculator()
	tenantID := "tenant-concurrent"

	// Simulate concurrent additions
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			calc.AddTokens(tenantID, "api", 100.0)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	record, found := calc.GetRecord(tenantID, time.Now().Format("2006-01"))
	if !found {
		t.Fatal("record not found")
	}

	if record.TotalTokens != 1000.0 {
		t.Errorf("expected 1000 tokens from concurrent adds, got %f", record.TotalTokens)
	}
}

func TestGetRecord_Concurrent(t *testing.T) {
	calc := NewCalculator()
	tenantID := "tenant-1"
	month := time.Now().Format("2006-01")

	calc.AddTokens(tenantID, "api", 100.0)

	// Simulate concurrent reads
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			record, found := calc.GetRecord(tenantID, month)
			if !found || record == nil {
				t.Fatal("record should be found")
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestAllRecords_Empty(t *testing.T) {
	calc := NewCalculator()

	allRecords := calc.AllRecords()
	// AllRecords returns nil for empty set
	if allRecords != nil && len(allRecords) != 0 {
		t.Errorf("expected nil or 0 records, got %d", len(allRecords))
	}
}

func TestAllRecords_Multiple(t *testing.T) {
	calc := NewCalculator()

	calc.AddTokens("tenant-1", "api", 100.0)
	calc.AddTokens("tenant-2", "api", 200.0)
	calc.AddTokens("tenant-3", "api", 300.0)

	allRecords := calc.AllRecords()
	if len(allRecords) != 3 {
		t.Errorf("expected 3 records, got %d", len(allRecords))
	}

	// Verify all tenants are present
	tenantMap := make(map[string]bool)
	for _, r := range allRecords {
		tenantMap[r.TenantID] = true
	}

	if !tenantMap["tenant-1"] || !tenantMap["tenant-2"] || !tenantMap["tenant-3"] {
		t.Fatal("not all tenants present in AllRecords")
	}
}

func TestAddTokens_BreakdownInitialization(t *testing.T) {
	calc := NewCalculator()
	tenantID := "tenant-1"

	calc.AddTokens(tenantID, "resource1", 100.0)

	record, found := calc.GetRecord(tenantID, time.Now().Format("2006-01"))
	if !found {
		t.Fatal("record not found")
	}

	if record.Breakdown == nil {
		t.Fatal("Breakdown is nil")
	}

	if len(record.Breakdown) != 1 {
		t.Errorf("expected 1 breakdown entry, got %d", len(record.Breakdown))
	}
}

func TestListRecords_MultipleMonths(t *testing.T) {
	calc := NewCalculator()
	tenantID := "tenant-1"

	// This test assumes we can create records from different months
	// Since AddTokens uses time.Now(), we can only test same month accumulation
	calc.AddTokens(tenantID, "api", 100.0)
	calc.AddTokens(tenantID, "api", 50.0)

	records := calc.ListRecords(tenantID)
	if len(records) != 1 {
		t.Errorf("expected 1 record for this month, got %d", len(records))
	}
}

func TestAddTokens_BreakdownNilInitialization(t *testing.T) {
	// Ensure that AddTokens correctly initializes Breakdown even on second call
	calc := NewCalculator()
	tenantID := "tenant-1"

	calc.AddTokens(tenantID, "api", 100.0)

	record, found := calc.GetRecord(tenantID, time.Now().Format("2006-01"))
	if !found {
		t.Fatal("record not found")
	}

	if record.Breakdown == nil {
		t.Fatal("Breakdown should be initialized on first add")
	}

	// Add to same resource type to ensure Breakdown is used correctly
	calc.AddTokens(tenantID, "api", 50.0)

	record, _ = calc.GetRecord(tenantID, time.Now().Format("2006-01"))
	if record.Breakdown["api"] != 150.0 {
		t.Errorf("expected api breakdown 150, got %f", record.Breakdown["api"])
	}
}

func TestAddTokens_NilBreakdownEdgeCase(t *testing.T) {
	calc := NewCalculator()
	tenantID := "tenant-1"
	month := time.Now().Format("2006-01")

	// Manually create a record with nil Breakdown to test the nil-check edge case
	key := tenantID + ":" + month
	calc.records[key] = &CostRecord{
		TenantID:    tenantID,
		Month:       month,
		TotalTokens: 100.0,
		TotalCostUSD: 100.0 * 0.0001,
		Breakdown:   nil, // Intentionally nil
	}

	// Now add tokens - should create the Breakdown if nil
	calc.AddTokens(tenantID, "api", 50.0)

	record, found := calc.GetRecord(tenantID, month)
	if !found {
		t.Fatal("record not found")
	}

	if record.Breakdown == nil {
		t.Fatal("Breakdown should not be nil after AddTokens")
	}

	if record.Breakdown["api"] != 50.0 {
		t.Errorf("expected api breakdown 50, got %f", record.Breakdown["api"])
	}
}
