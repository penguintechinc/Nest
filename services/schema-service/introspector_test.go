package main

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

func getLogger() *zap.Logger {
	logger, _ := zap.NewDevelopment()
	return logger
}

func TestNewIntrospector(t *testing.T) {
	logger := getLogger()
	defer logger.Sync()

	introspector := NewIntrospector(logger)
	if introspector == nil {
		t.Fatal("NewIntrospector returned nil")
	}

	if introspector.logger != logger {
		t.Fatal("logger not set correctly")
	}
}

func TestIntrospect_Postgres(t *testing.T) {
	logger := getLogger()
	defer logger.Sync()

	introspector := NewIntrospector(logger)
	ctx := context.Background()

	schema, err := introspector.Introspect(ctx, "pg-db-1", "postgres", "localhost:5432")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if schema == nil {
		t.Fatal("returned schema is nil")
	}

	if schema.ResourceID != "pg-db-1" {
		t.Errorf("expected resourceID 'pg-db-1', got '%s'", schema.ResourceID)
	}

	if schema.ResourceType != "postgres" {
		t.Errorf("expected resourceType 'postgres', got '%s'", schema.ResourceType)
	}

	if len(schema.Fields) == 0 {
		t.Fatal("expected fields in postgres schema")
	}

	// Check for expected postgres fields
	hasID := false
	hasCreatedAt := false

	for _, field := range schema.Fields {
		if field.Name == "id" && field.Type == "bigint" && !field.Nullable && field.Primary {
			hasID = true
		}

		if field.Name == "created_at" && field.Type == "timestamptz" && field.Nullable && !field.Primary {
			hasCreatedAt = true
		}
	}

	if !hasID {
		t.Error("expected 'id' field in postgres schema")
	}

	if !hasCreatedAt {
		t.Error("expected 'created_at' field in postgres schema")
	}

	if len(schema.Indexes) == 0 {
		t.Fatal("expected indexes in postgres schema")
	}

	// Check for expected postgres index
	foundPKey := false
	for _, idx := range schema.Indexes {
		if idx.Name == "pkey" && idx.Unique {
			foundPKey = true
		}
	}

	if !foundPKey {
		t.Error("expected 'pkey' index in postgres schema")
	}

	if schema.TableCount != 0 {
		t.Errorf("expected tableCount 0, got %d", schema.TableCount)
	}
}

func TestIntrospect_MySQL(t *testing.T) {
	logger := getLogger()
	defer logger.Sync()

	introspector := NewIntrospector(logger)
	ctx := context.Background()

	schema, err := introspector.Introspect(ctx, "mysql-db-1", "mysql", "localhost:3306")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if schema == nil {
		t.Fatal("returned schema is nil")
	}

	if schema.ResourceType != "mysql" {
		t.Errorf("expected resourceType 'mysql', got '%s'", schema.ResourceType)
	}

	// Check for expected mysql fields
	hasID := false
	hasCreatedAt := false

	for _, field := range schema.Fields {
		if field.Name == "id" && field.Type == "bigint" && !field.Nullable && field.Primary {
			hasID = true
		}

		if field.Name == "created_at" && field.Type == "datetime" && field.Nullable && !field.Primary {
			hasCreatedAt = true
		}
	}

	if !hasID {
		t.Error("expected 'id' field in mysql schema")
	}

	if !hasCreatedAt {
		t.Error("expected 'created_at' field in mysql schema")
	}

	// Check for PRIMARY index
	foundPrimary := false
	for _, idx := range schema.Indexes {
		if idx.Name == "PRIMARY" && idx.Unique {
			foundPrimary = true
		}
	}

	if !foundPrimary {
		t.Error("expected 'PRIMARY' index in mysql schema")
	}
}

func TestIntrospect_MariaDB(t *testing.T) {
	logger := getLogger()
	defer logger.Sync()

	introspector := NewIntrospector(logger)
	ctx := context.Background()

	schema, err := introspector.Introspect(ctx, "mariadb-1", "mariadb", "localhost:3306")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if schema == nil {
		t.Fatal("returned schema is nil")
	}

	if schema.ResourceType != "mariadb" {
		t.Errorf("expected resourceType 'mariadb', got '%s'", schema.ResourceType)
	}

	// MariaDB should return similar structure to MySQL
	if len(schema.Fields) == 0 {
		t.Fatal("expected fields in mariadb schema")
	}

	if len(schema.Indexes) == 0 {
		t.Fatal("expected indexes in mariadb schema")
	}
}

func TestIntrospect_ClickHouse(t *testing.T) {
	logger := getLogger()
	defer logger.Sync()

	introspector := NewIntrospector(logger)
	ctx := context.Background()

	schema, err := introspector.Introspect(ctx, "ch-cluster-1", "clickhouse", "localhost:9000")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if schema == nil {
		t.Fatal("returned schema is nil")
	}

	if schema.ResourceType != "clickhouse" {
		t.Errorf("expected resourceType 'clickhouse', got '%s'", schema.ResourceType)
	}

	// Check for expected clickhouse fields
	hasEventTime := false
	hasValue := false

	for _, field := range schema.Fields {
		if field.Name == "event_time" && field.Type == "DateTime" && !field.Nullable && !field.Primary {
			hasEventTime = true
		}

		if field.Name == "value" && field.Type == "Float64" && field.Nullable && !field.Primary {
			hasValue = true
		}
	}

	if !hasEventTime {
		t.Error("expected 'event_time' field in clickhouse schema")
	}

	if !hasValue {
		t.Error("expected 'value' field in clickhouse schema")
	}

	if schema.TableCount != 0 {
		t.Errorf("expected tableCount 0, got %d", schema.TableCount)
	}
}

func TestIntrospect_Search(t *testing.T) {
	logger := getLogger()
	defer logger.Sync()

	introspector := NewIntrospector(logger)
	ctx := context.Background()

	schema, err := introspector.Introspect(ctx, "es-cluster-1", "search", "localhost:9200")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if schema == nil {
		t.Fatal("returned schema is nil")
	}

	if schema.ResourceType != "search" {
		t.Errorf("expected resourceType 'search', got '%s'", schema.ResourceType)
	}

	// Check for expected elasticsearch fields
	hasID := false
	hasContent := false

	for _, field := range schema.Fields {
		if field.Name == "_id" && field.Type == "keyword" && !field.Nullable && field.Primary {
			hasID = true
		}

		if field.Name == "content" && field.Type == "text" && field.Nullable && !field.Primary {
			hasContent = true
		}
	}

	if !hasID {
		t.Error("expected '_id' field in search schema")
	}

	if !hasContent {
		t.Error("expected 'content' field in search schema")
	}

	// Check for indexCount in extras
	if indexCount, ok := schema.Extra["indexCount"]; !ok {
		t.Error("expected 'indexCount' in search schema extras")
	} else if indexCount != 0 {
		t.Errorf("expected indexCount 0, got %v", indexCount)
	}
}

func TestIntrospect_Kafka(t *testing.T) {
	logger := getLogger()
	defer logger.Sync()

	introspector := NewIntrospector(logger)
	ctx := context.Background()

	schema, err := introspector.Introspect(ctx, "kafka-cluster-1", "kafka", "localhost:9092")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if schema == nil {
		t.Fatal("returned schema is nil")
	}

	if schema.ResourceType != "kafka" {
		t.Errorf("expected resourceType 'kafka', got '%s'", schema.ResourceType)
	}

	// Check for expected kafka fields
	hasKey := false
	hasValue := false
	hasOffset := false

	for _, field := range schema.Fields {
		if field.Name == "key" && field.Type == "bytes" && field.Nullable && !field.Primary {
			hasKey = true
		}

		if field.Name == "value" && field.Type == "bytes" && !field.Nullable && !field.Primary {
			hasValue = true
		}

		if field.Name == "offset" && field.Type == "int64" && !field.Nullable && !field.Primary {
			hasOffset = true
		}
	}

	if !hasKey {
		t.Error("expected 'key' field in kafka schema")
	}

	if !hasValue {
		t.Error("expected 'value' field in kafka schema")
	}

	if !hasOffset {
		t.Error("expected 'offset' field in kafka schema")
	}

	// Check for topicCount in extras
	if topicCount, ok := schema.Extra["topicCount"]; !ok {
		t.Error("expected 'topicCount' in kafka schema extras")
	} else if topicCount != 0 {
		t.Errorf("expected topicCount 0, got %v", topicCount)
	}
}

func TestIntrospect_UnknownType(t *testing.T) {
	logger := getLogger()
	defer logger.Sync()

	introspector := NewIntrospector(logger)
	ctx := context.Background()

	schema, err := introspector.Introspect(ctx, "unknown-1", "unknown_type", "localhost:9999")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if schema == nil {
		t.Fatal("returned schema is nil")
	}

	if schema.ResourceType != "unknown_type" {
		t.Errorf("expected resourceType 'unknown_type', got '%s'", schema.ResourceType)
	}

	// Should return default schema
	if len(schema.Fields) == 0 {
		t.Fatal("expected default fields for unknown type")
	}

	// Check for default id field
	hasID := false
	for _, field := range schema.Fields {
		if field.Name == "id" && field.Type == "string" && !field.Nullable && field.Primary {
			hasID = true
		}
	}

	if !hasID {
		t.Error("expected default 'id' field for unknown type")
	}
}

func TestIntrospect_DiscoveredAtSet(t *testing.T) {
	logger := getLogger()
	defer logger.Sync()

	introspector := NewIntrospector(logger)
	ctx := context.Background()

	schema, _ := introspector.Introspect(ctx, "test-1", "postgres", "localhost:5432")

	if schema.DiscoveredAt.IsZero() {
		t.Fatal("DiscoveredAt not set")
	}
}

func TestIntrospect_ExtraMapInitialized(t *testing.T) {
	logger := getLogger()
	defer logger.Sync()

	introspector := NewIntrospector(logger)
	ctx := context.Background()

	schema, _ := introspector.Introspect(ctx, "test-1", "postgres", "localhost:5432")

	if schema.Extra == nil {
		t.Fatal("Extra map is nil")
	}
}

func TestIntrospect_IndexesInitialized(t *testing.T) {
	logger := getLogger()
	defer logger.Sync()

	introspector := NewIntrospector(logger)
	ctx := context.Background()

	schema, _ := introspector.Introspect(ctx, "test-1", "postgres", "localhost:5432")

	if schema.Indexes == nil {
		t.Fatal("Indexes is nil")
	}

	if len(schema.Indexes) == 0 {
		t.Fatal("expected indexes for postgres")
	}
}

func TestIntrospect_PostgresFieldDetails(t *testing.T) {
	logger := getLogger()
	defer logger.Sync()

	introspector := NewIntrospector(logger)
	ctx := context.Background()

	schema, _ := introspector.Introspect(ctx, "pg-1", "postgres", "localhost:5432")

	// Verify all postgres field details
	if len(schema.Fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(schema.Fields))
	}

	idField := schema.Fields[0]
	if idField.Name != "id" || idField.Type != "bigint" || idField.Nullable || !idField.Primary {
		t.Error("id field not configured correctly")
	}

	createdAtField := schema.Fields[1]
	if createdAtField.Name != "created_at" || createdAtField.Type != "timestamptz" || !createdAtField.Nullable || createdAtField.Primary {
		t.Error("created_at field not configured correctly")
	}
}

func TestIntrospect_MySQLFieldDetails(t *testing.T) {
	logger := getLogger()
	defer logger.Sync()

	introspector := NewIntrospector(logger)
	ctx := context.Background()

	schema, _ := introspector.Introspect(ctx, "mysql-1", "mysql", "localhost:3306")

	// Verify all mysql field details
	if len(schema.Fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(schema.Fields))
	}

	idField := schema.Fields[0]
	if idField.Name != "id" || idField.Type != "bigint" || idField.Nullable || !idField.Primary {
		t.Error("id field not configured correctly")
	}

	createdAtField := schema.Fields[1]
	if createdAtField.Name != "created_at" || createdAtField.Type != "datetime" || !createdAtField.Nullable || createdAtField.Primary {
		t.Error("created_at field not configured correctly")
	}
}

func TestIntrospect_ClickHouseFieldDetails(t *testing.T) {
	logger := getLogger()
	defer logger.Sync()

	introspector := NewIntrospector(logger)
	ctx := context.Background()

	schema, _ := introspector.Introspect(ctx, "ch-1", "clickhouse", "localhost:9000")

	// Verify all clickhouse field details
	if len(schema.Fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(schema.Fields))
	}

	eventTimeField := schema.Fields[0]
	if eventTimeField.Name != "event_time" || eventTimeField.Type != "DateTime" || eventTimeField.Nullable || eventTimeField.Primary {
		t.Error("event_time field not configured correctly")
	}

	valueField := schema.Fields[1]
	if valueField.Name != "value" || valueField.Type != "Float64" || !valueField.Nullable || valueField.Primary {
		t.Error("value field not configured correctly")
	}
}

func TestIntrospect_KafkaFieldDetails(t *testing.T) {
	logger := getLogger()
	defer logger.Sync()

	introspector := NewIntrospector(logger)
	ctx := context.Background()

	schema, _ := introspector.Introspect(ctx, "kafka-1", "kafka", "localhost:9092")

	// Verify all kafka field details
	if len(schema.Fields) != 3 {
		t.Errorf("expected 3 fields, got %d", len(schema.Fields))
	}

	keyField := schema.Fields[0]
	if keyField.Name != "key" || keyField.Type != "bytes" || !keyField.Nullable || keyField.Primary {
		t.Error("key field not configured correctly")
	}

	valueField := schema.Fields[1]
	if valueField.Name != "value" || valueField.Type != "bytes" || valueField.Nullable || valueField.Primary {
		t.Error("value field not configured correctly")
	}

	offsetField := schema.Fields[2]
	if offsetField.Name != "offset" || offsetField.Type != "int64" || offsetField.Nullable || offsetField.Primary {
		t.Error("offset field not configured correctly")
	}
}

func TestIntrospect_PostgresIndexDetails(t *testing.T) {
	logger := getLogger()
	defer logger.Sync()

	introspector := NewIntrospector(logger)
	ctx := context.Background()

	schema, _ := introspector.Introspect(ctx, "pg-1", "postgres", "localhost:5432")

	if len(schema.Indexes) != 1 {
		t.Errorf("expected 1 index, got %d", len(schema.Indexes))
	}

	pkeyIndex := schema.Indexes[0]
	if pkeyIndex.Name != "pkey" || !pkeyIndex.Unique {
		t.Error("pkey index not configured correctly")
	}

	if len(pkeyIndex.Fields) != 1 || pkeyIndex.Fields[0] != "id" {
		t.Error("pkey index fields not configured correctly")
	}
}

func TestIntrospect_SearchExtraMap(t *testing.T) {
	logger := getLogger()
	defer logger.Sync()

	introspector := NewIntrospector(logger)
	ctx := context.Background()

	schema, _ := introspector.Introspect(ctx, "es-1", "search", "localhost:9200")

	if len(schema.Extra) == 0 {
		t.Error("expected extra map for search to have entries")
	}

	if indexCount, ok := schema.Extra["indexCount"]; ok {
		if indexCount.(int) != 0 {
			t.Errorf("expected indexCount 0, got %v", indexCount)
		}
	} else {
		t.Error("expected 'indexCount' key in extras")
	}
}

func TestIntrospect_KafkaExtraMap(t *testing.T) {
	logger := getLogger()
	defer logger.Sync()

	introspector := NewIntrospector(logger)
	ctx := context.Background()

	schema, _ := introspector.Introspect(ctx, "kafka-1", "kafka", "localhost:9092")

	if len(schema.Extra) == 0 {
		t.Error("expected extra map for kafka to have entries")
	}

	if topicCount, ok := schema.Extra["topicCount"]; ok {
		if topicCount.(int) != 0 {
			t.Errorf("expected topicCount 0, got %v", topicCount)
		}
	} else {
		t.Error("expected 'topicCount' key in extras")
	}
}

func TestIntrospect_ResourceIDPreserved(t *testing.T) {
	logger := getLogger()
	defer logger.Sync()

	introspector := NewIntrospector(logger)
	ctx := context.Background()

	testCases := []string{
		"pg-db-1",
		"mysql-cluster-prod",
		"es-index-v2",
		"kafka-topic-events",
		"ch-distributed",
	}

	for _, resourceID := range testCases {
		schema, _ := introspector.Introspect(ctx, resourceID, "postgres", "localhost")
		if schema.ResourceID != resourceID {
			t.Errorf("expected resourceID '%s', got '%s'", resourceID, schema.ResourceID)
		}
	}
}

func TestIntrospect_EndpointPreserved(t *testing.T) {
	logger := getLogger()
	defer logger.Sync()

	introspector := NewIntrospector(logger)
	ctx := context.Background()

	testEndpoints := []string{
		"localhost:5432",
		"db.example.com:3306",
		"elasticsearch:9200",
		"kafka-broker:9092",
	}

	for _, endpoint := range testEndpoints {
		schema, _ := introspector.Introspect(ctx, "test", "postgres", endpoint)
		if schema == nil {
			t.Errorf("expected non-nil schema for endpoint '%s'", endpoint)
		}
	}
}

func TestIntrospect_ContextCancelled(t *testing.T) {
	logger := getLogger()
	defer logger.Sync()

	introspector := NewIntrospector(logger)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Should still work (introspection is synchronous, doesn't check context)
	schema, err := introspector.Introspect(ctx, "test", "postgres", "localhost:5432")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if schema == nil {
		t.Fatal("expected non-nil schema even with cancelled context")
	}
}

func TestIntrospect_AllResourceTypes(t *testing.T) {
	logger := getLogger()
	defer logger.Sync()

	introspector := NewIntrospector(logger)
	ctx := context.Background()

	resourceTypes := []string{"postgres", "mysql", "mariadb", "clickhouse", "search", "kafka"}

	for _, resourceType := range resourceTypes {
		schema, err := introspector.Introspect(ctx, "test", resourceType, "localhost")
		if err != nil {
			t.Errorf("unexpected error for %s: %v", resourceType, err)
		}

		if schema == nil {
			t.Errorf("expected non-nil schema for %s", resourceType)
		}

		if schema.ResourceType != resourceType {
			t.Errorf("expected resourceType '%s', got '%s'", resourceType, schema.ResourceType)
		}

		if len(schema.Fields) == 0 {
			t.Errorf("expected fields for %s", resourceType)
		}
	}
}
