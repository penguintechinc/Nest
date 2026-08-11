package main

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func getTestLogger() *zap.Logger {
	logger, _ := zap.NewDevelopment()
	return logger
}

func TestNewCatalog(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()

	catalog := NewCatalog(logger)
	if catalog == nil {
		t.Fatal("NewCatalog returned nil")
	}
	if catalog.namespaces == nil {
		t.Fatal("namespaces map is nil")
	}
	if catalog.tables == nil {
		t.Fatal("tables map is nil")
	}
	if len(catalog.namespaces) != 0 {
		t.Errorf("expected empty namespaces, got %d", len(catalog.namespaces))
	}
	if len(catalog.tables) != 0 {
		t.Errorf("expected empty tables, got %d", len(catalog.tables))
	}
}

// --- Namespace Tests ---

func TestCreateNamespace_Success(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	ns, err := catalog.CreateNamespace("default", nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if ns == nil {
		t.Fatal("returned namespace is nil")
	}

	if ns.Name != "default" {
		t.Errorf("expected name 'default', got '%s'", ns.Name)
	}

	if ns.Properties == nil {
		t.Fatal("Properties is nil")
	}

	if ns.CreatedAt.IsZero() {
		t.Fatal("CreatedAt not set")
	}
}

func TestCreateNamespace_WithProperties(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	props := map[string]string{
		"owner": "data-team",
		"tier":  "production",
	}

	ns, err := catalog.CreateNamespace("analytics", props)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if ns.Properties["owner"] != "data-team" {
		t.Errorf("expected owner 'data-team', got '%s'", ns.Properties["owner"])
	}

	if ns.Properties["tier"] != "production" {
		t.Errorf("expected tier 'production', got '%s'", ns.Properties["tier"])
	}
}

func TestCreateNamespace_Duplicate(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	catalog.CreateNamespace("default", nil)

	// Try to create duplicate
	_, err := catalog.CreateNamespace("default", nil)
	if err == nil {
		t.Fatal("expected error for duplicate namespace")
	}

	if err.Error() != "namespace already exists: default" {
		t.Errorf("expected 'namespace already exists: default', got '%s'", err.Error())
	}
}

func TestListNamespaces_All(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	catalog.CreateNamespace("ns1", nil)
	catalog.CreateNamespace("ns2", nil)
	catalog.CreateNamespace("ns3", nil)

	namespaces := catalog.ListNamespaces("")
	if len(namespaces) != 3 {
		t.Errorf("expected 3 namespaces, got %d", len(namespaces))
	}
}

func TestListNamespaces_FilterByParent(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	catalog.CreateNamespace("default", nil)
	catalog.CreateNamespace("analytics", nil)

	namespaces := catalog.ListNamespaces("default")
	if len(namespaces) != 1 {
		t.Errorf("expected 1 namespace, got %d", len(namespaces))
	}

	if namespaces[0].Name != "default" {
		t.Errorf("expected 'default', got '%s'", namespaces[0].Name)
	}
}

func TestListNamespaces_Empty(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	namespaces := catalog.ListNamespaces("")
	if len(namespaces) != 0 {
		t.Errorf("expected 0 namespaces, got %d", len(namespaces))
	}
}

func TestGetNamespace_Found(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	catalog.CreateNamespace("default", nil)

	ns, err := catalog.GetNamespace("default")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if ns == nil {
		t.Fatal("returned namespace is nil")
	}

	if ns.Name != "default" {
		t.Errorf("expected 'default', got '%s'", ns.Name)
	}
}

func TestGetNamespace_NotFound(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	_, err := catalog.GetNamespace("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent namespace")
	}

	if err.Error() != "namespace not found: nonexistent" {
		t.Errorf("expected 'namespace not found: nonexistent', got '%s'", err.Error())
	}
}

func TestDropNamespace_Success(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	catalog.CreateNamespace("default", nil)

	err := catalog.DropNamespace("default")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify it's gone
	_, err = catalog.GetNamespace("default")
	if err == nil {
		t.Fatal("expected error after dropping namespace")
	}
}

func TestDropNamespace_NotFound(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	err := catalog.DropNamespace("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent namespace")
	}

	if err.Error() != "namespace not found: nonexistent" {
		t.Errorf("expected 'namespace not found: nonexistent', got '%s'", err.Error())
	}
}

func TestDropNamespace_NotEmpty(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	catalog.CreateNamespace("default", nil)
	catalog.CreateTable("default", "my_table", "s3://bucket/table", nil, nil)

	err := catalog.DropNamespace("default")
	if err == nil {
		t.Fatal("expected error for non-empty namespace")
	}

	if err.Error() != "namespace not empty: default" {
		t.Errorf("expected 'namespace not empty: default', got '%s'", err.Error())
	}
}

// --- Table Tests ---

func TestCreateTable_Success(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	catalog.CreateNamespace("default", nil)

	table, err := catalog.CreateTable("default", "users", "s3://bucket/users", nil, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if table == nil {
		t.Fatal("returned table is nil")
	}

	if table.Name != "users" {
		t.Errorf("expected name 'users', got '%s'", table.Name)
	}

	if table.Namespace != "default" {
		t.Errorf("expected namespace 'default', got '%s'", table.Namespace)
	}

	if table.Location != "s3://bucket/users" {
		t.Errorf("expected location 's3://bucket/users', got '%s'", table.Location)
	}

	if table.MetadataLocation != "s3://bucket/users/.iceberg/metadata/v0.json" {
		t.Errorf("unexpected metadata location: %s", table.MetadataLocation)
	}
}

func TestCreateTable_WithSchema(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	catalog.CreateNamespace("default", nil)

	schema := map[string]interface{}{
		"type":   "struct",
		"fields": []interface{}{},
	}

	table, err := catalog.CreateTable("default", "events", "s3://bucket/events", schema, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if table.Schema == nil {
		t.Fatal("schema is nil")
	}

	if table.Schema["type"] != "struct" {
		t.Errorf("expected schema type 'struct', got '%v'", table.Schema["type"])
	}
}

func TestCreateTable_WithProperties(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	catalog.CreateNamespace("default", nil)

	props := map[string]string{
		"format": "parquet",
		"owner":  "data-team",
	}

	table, err := catalog.CreateTable("default", "products", "s3://bucket/products", nil, props)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if table.Properties["format"] != "parquet" {
		t.Errorf("expected format 'parquet', got '%s'", table.Properties["format"])
	}

	if table.Properties["owner"] != "data-team" {
		t.Errorf("expected owner 'data-team', got '%s'", table.Properties["owner"])
	}
}

func TestCreateTable_NamespaceNotFound(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	_, err := catalog.CreateTable("nonexistent", "users", "s3://bucket/users", nil, nil)
	if err == nil {
		t.Fatal("expected error for nonexistent namespace")
	}

	if err.Error() != "namespace not found: nonexistent" {
		t.Errorf("expected 'namespace not found: nonexistent', got '%s'", err.Error())
	}
}

func TestCreateTable_Duplicate(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	catalog.CreateNamespace("default", nil)
	catalog.CreateTable("default", "users", "s3://bucket/users", nil, nil)

	// Try to create duplicate
	_, err := catalog.CreateTable("default", "users", "s3://bucket/users-2", nil, nil)
	if err == nil {
		t.Fatal("expected error for duplicate table")
	}

	if err.Error() != "table already exists: default.users" {
		t.Errorf("expected 'table already exists: default.users', got '%s'", err.Error())
	}
}

func TestListTables_Success(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	catalog.CreateNamespace("default", nil)
	catalog.CreateTable("default", "users", "s3://bucket/users", nil, nil)
	catalog.CreateTable("default", "orders", "s3://bucket/orders", nil, nil)
	catalog.CreateTable("default", "products", "s3://bucket/products", nil, nil)

	tables, err := catalog.ListTables("default")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(tables) != 3 {
		t.Errorf("expected 3 tables, got %d", len(tables))
	}
}

func TestListTables_NamespaceNotFound(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	_, err := catalog.ListTables("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent namespace")
	}

	if err.Error() != "namespace not found: nonexistent" {
		t.Errorf("expected 'namespace not found: nonexistent', got '%s'", err.Error())
	}
}

func TestListTables_Empty(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	catalog.CreateNamespace("default", nil)

	tables, err := catalog.ListTables("default")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(tables) != 0 {
		t.Errorf("expected 0 tables, got %d", len(tables))
	}
}

func TestListTables_MultipleNamespaces(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	catalog.CreateNamespace("default", nil)
	catalog.CreateNamespace("analytics", nil)

	catalog.CreateTable("default", "users", "s3://bucket/users", nil, nil)
	catalog.CreateTable("analytics", "events", "s3://bucket/events", nil, nil)

	defaultTables, _ := catalog.ListTables("default")
	analyticsTables, _ := catalog.ListTables("analytics")

	if len(defaultTables) != 1 {
		t.Errorf("expected 1 table in default namespace, got %d", len(defaultTables))
	}

	if len(analyticsTables) != 1 {
		t.Errorf("expected 1 table in analytics namespace, got %d", len(analyticsTables))
	}
}

func TestGetTable_Found(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	catalog.CreateNamespace("default", nil)
	catalog.CreateTable("default", "users", "s3://bucket/users", nil, nil)

	table, err := catalog.GetTable("default", "users")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if table == nil {
		t.Fatal("returned table is nil")
	}

	if table.Name != "users" {
		t.Errorf("expected name 'users', got '%s'", table.Name)
	}
}

func TestGetTable_NotFound(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	catalog.CreateNamespace("default", nil)

	_, err := catalog.GetTable("default", "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent table")
	}

	if err.Error() != "table not found: default.nonexistent" {
		t.Errorf("expected 'table not found: default.nonexistent', got '%s'", err.Error())
	}
}

func TestDropTable_Success(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	catalog.CreateNamespace("default", nil)
	catalog.CreateTable("default", "users", "s3://bucket/users", nil, nil)

	err := catalog.DropTable("default", "users")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify it's gone
	_, err = catalog.GetTable("default", "users")
	if err == nil {
		t.Fatal("expected error after dropping table")
	}
}

func TestDropTable_NotFound(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	catalog.CreateNamespace("default", nil)

	err := catalog.DropTable("default", "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent table")
	}

	if err.Error() != "table not found: default.nonexistent" {
		t.Errorf("expected 'table not found: default.nonexistent', got '%s'", err.Error())
	}
}

func TestUpdateTable_Success(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	catalog.CreateNamespace("default", nil)
	catalog.CreateTable("default", "users", "s3://bucket/users", nil, nil)

	updates := map[string]interface{}{
		"location": "s3://bucket/users-v2",
	}

	table, err := catalog.UpdateTable("default", "users", updates)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if table.Location != "s3://bucket/users-v2" {
		t.Errorf("expected location 's3://bucket/users-v2', got '%s'", table.Location)
	}
}

func TestUpdateTable_UpdateSchema(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	catalog.CreateNamespace("default", nil)
	oldSchema := map[string]interface{}{
		"version": 1,
	}
	catalog.CreateTable("default", "users", "s3://bucket/users", oldSchema, nil)

	newSchema := map[string]interface{}{
		"version": 2,
		"fields":  []interface{}{},
	}

	updates := map[string]interface{}{
		"schema": newSchema,
	}

	table, err := catalog.UpdateTable("default", "users", updates)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Compare version as interface{} since it could be int or float64
	if table.Schema["version"] != 2 {
		t.Errorf("expected schema version 2, got %v", table.Schema["version"])
	}
}

func TestUpdateTable_UpdateProperties(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	catalog.CreateNamespace("default", nil)
	props := map[string]string{"owner": "team-a"}
	catalog.CreateTable("default", "users", "s3://bucket/users", nil, props)

	updates := map[string]interface{}{
		"properties": map[string]string{"owner": "team-b", "tier": "premium"},
	}

	table, err := catalog.UpdateTable("default", "users", updates)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if table.Properties["owner"] != "team-b" {
		t.Errorf("expected owner 'team-b', got '%s'", table.Properties["owner"])
	}

	if table.Properties["tier"] != "premium" {
		t.Errorf("expected tier 'premium', got '%s'", table.Properties["tier"])
	}
}

func TestUpdateTable_NotFound(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	catalog.CreateNamespace("default", nil)

	_, err := catalog.UpdateTable("default", "nonexistent", map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for nonexistent table")
	}

	if err.Error() != "table not found: default.nonexistent" {
		t.Errorf("expected 'table not found: default.nonexistent', got '%s'", err.Error())
	}
}

func TestUpdateTable_UpdatedAtChanges(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	catalog.CreateNamespace("default", nil)
	table1, _ := catalog.CreateTable("default", "users", "s3://bucket/users", nil, nil)
	originalUpdatedAt := table1.UpdatedAt

	// Wait a tiny bit to ensure different timestamps
	time.Sleep(1 * time.Millisecond)

	updates := map[string]interface{}{
		"location": "s3://bucket/users-v2",
	}

	table2, _ := catalog.UpdateTable("default", "users", updates)

	if !table2.UpdatedAt.After(originalUpdatedAt) {
		t.Fatal("UpdatedAt should be changed after update")
	}
}

func TestCreateTable_TimestampsSet(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	catalog.CreateNamespace("default", nil)

	before := time.Now().UTC()
	table, _ := catalog.CreateTable("default", "users", "s3://bucket/users", nil, nil)
	after := time.Now().UTC()

	if table.CreatedAt.Before(before) || table.CreatedAt.After(after.Add(1*time.Second)) {
		t.Fatal("CreatedAt not set correctly")
	}

	if table.UpdatedAt.Before(before) || table.UpdatedAt.After(after.Add(1*time.Second)) {
		t.Fatal("UpdatedAt not set correctly")
	}
}

func TestCatalog_Concurrent(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	catalog.CreateNamespace("default", nil)

	done := make(chan bool)

	// Simulate concurrent table operations
	for i := 1; i <= 5; i++ {
		go func(id int) {
			tableName := "table_" + string(rune('a'+id-1))
			catalog.CreateTable("default", tableName, "s3://bucket/"+tableName, nil, nil)
			done <- true
		}(i)
	}

	for i := 0; i < 5; i++ {
		<-done
	}

	tables, _ := catalog.ListTables("default")
	if len(tables) != 5 {
		t.Errorf("expected 5 tables from concurrent creates, got %d", len(tables))
	}
}

func TestNamespace_CreatedAtUTC(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	ns, _ := catalog.CreateNamespace("default", nil)

	// CreatedAt should be in UTC
	if ns.CreatedAt.Location() != time.UTC {
		t.Errorf("expected CreatedAt in UTC, got %v", ns.CreatedAt.Location())
	}
}

func TestTable_CreatedAtUTC(t *testing.T) {
	logger := getTestLogger()
	defer logger.Sync()
	catalog := NewCatalog(logger)

	catalog.CreateNamespace("default", nil)
	table, _ := catalog.CreateTable("default", "users", "s3://bucket/users", nil, nil)

	// CreatedAt and UpdatedAt should be in UTC
	if table.CreatedAt.Location() != time.UTC {
		t.Errorf("expected CreatedAt in UTC, got %v", table.CreatedAt.Location())
	}

	if table.UpdatedAt.Location() != time.UTC {
		t.Errorf("expected UpdatedAt in UTC, got %v", table.UpdatedAt.Location())
	}
}
