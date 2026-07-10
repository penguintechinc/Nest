package protocol

import (
	"testing"
)

func TestClassifyQuery(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected QueryType
	}{
		{"select", "SELECT * FROM users", QueryTypeSelect},
		{"select with spaces", "  SELECT * FROM users  ", QueryTypeSelect},
		{"select uppercase", "select * from users", QueryTypeSelect},
		{"select with CTE", "WITH cte AS (SELECT 1) SELECT * FROM cte", QueryTypeSelect},
		{"insert", "INSERT INTO users (name) VALUES ('alice')", QueryTypeInsert},
		{"update", "UPDATE users SET name='bob' WHERE id=1", QueryTypeUpdate},
		{"delete", "DELETE FROM users WHERE id=1", QueryTypeDelete},
		{"create table", "CREATE TABLE users (id INT)", QueryTypeDDL},
		{"alter table", "ALTER TABLE users ADD COLUMN email VARCHAR(255)", QueryTypeDDL},
		{"drop table", "DROP TABLE users", QueryTypeDDL},
		{"truncate", "TRUNCATE TABLE users", QueryTypeDDL},
		{"call", "CALL proc_name()", QueryTypeCall},
		{"begin", "BEGIN", QueryTypeBegin},
		{"begin transaction", "BEGIN TRANSACTION", QueryTypeBegin},
		{"start transaction", "START TRANSACTION", QueryTypeBegin},
		{"commit", "COMMIT", QueryTypeCommit},
		{"rollback", "ROLLBACK", QueryTypeRollback},
		{"show", "SHOW TABLES", QueryTypeSelect},
		{"explain", "EXPLAIN SELECT * FROM users", QueryTypeSelect},
		{"describe", "DESCRIBE users", QueryTypeSelect},
		{"empty", "", QueryTypeUnknown},
		{"whitespace only", "   ", QueryTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyQuery(tt.query)
			if result != tt.expected {
				t.Errorf("got %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestQueryTypeIsRead(t *testing.T) {
	tests := []struct {
		queryType QueryType
		expected  bool
	}{
		{QueryTypeSelect, true},
		{QueryTypeInsert, false},
		{QueryTypeUpdate, false},
		{QueryTypeDelete, false},
		{QueryTypeDDL, false},
		{QueryTypeCall, false},
		{QueryTypeUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.queryType.String(), func(t *testing.T) {
			result := tt.queryType.IsRead()
			if result != tt.expected {
				t.Errorf("got %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestQueryTypeIsWrite(t *testing.T) {
	tests := []struct {
		queryType QueryType
		expected  bool
	}{
		{QueryTypeSelect, false},
		{QueryTypeInsert, true},
		{QueryTypeUpdate, true},
		{QueryTypeDelete, true},
		{QueryTypeDDL, true},
		{QueryTypeCall, true},
		{QueryTypeUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.queryType.String(), func(t *testing.T) {
			result := tt.queryType.IsWrite()
			if result != tt.expected {
				t.Errorf("got %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestTrimAndNormalize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"SELECT * FROM users", "SELECT * FROM users"},
		{"  SELECT * FROM users  ", "SELECT * FROM users"},
		{"-- comment\nSELECT * FROM users", "SELECT * FROM users"},
		{"/* block comment */ SELECT * FROM users", "SELECT * FROM users"},
		{"", ""},
		{"-- comment only", ""},
		{"   ", ""},
	}

	for _, tt := range tests {
		result := TrimAndNormalize(tt.input)
		if result != tt.expected {
			t.Errorf("input: %q, got %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestMySQLParserParse(t *testing.T) {
	parser := &MySQLParser{}

	tests := []struct {
		name          string
		data          []byte
		expectedType  QueryType
		expectedError bool
	}{
		{
			name:         "COM_QUERY with SELECT",
			data:         append([]byte{0x00, 0x00, 0x00, 0x00, 0x03}, []byte("SELECT * FROM users")...),
			expectedType: QueryTypeSelect,
		},
		{
			name:         "COM_QUERY with INSERT",
			data:         append([]byte{0x00, 0x00, 0x00, 0x00, 0x03}, []byte("INSERT INTO users VALUES (1)")...),
			expectedType: QueryTypeInsert,
		},
		{
			name:         "too short",
			data:         []byte{0x00, 0x00},
			expectedType: QueryTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.Parse(tt.data)
			if result.QueryType != tt.expectedType {
				t.Errorf("got %v, expected %v", result.QueryType, tt.expectedType)
			}
			if (err != nil) != tt.expectedError {
				t.Errorf("error: got %v, expected error: %v", err, tt.expectedError)
			}
		})
	}
}

func TestPostgreSQLParserParse(t *testing.T) {
	parser := &PostgreSQLParser{}

	tests := []struct {
		name          string
		data          []byte
		expectedType  QueryType
		expectedError bool
	}{
		{
			name:         "simple query SELECT",
			data:         buildPgQuery("SELECT * FROM users"),
			expectedType: QueryTypeSelect,
		},
		{
			name:         "simple query INSERT",
			data:         buildPgQuery("INSERT INTO users VALUES (1)"),
			expectedType: QueryTypeInsert,
		},
		{
			name:         "too short",
			data:         []byte{0x00, 0x00},
			expectedType: QueryTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.Parse(tt.data)
			if result.QueryType != tt.expectedType {
				t.Errorf("got %v, expected %v", result.QueryType, tt.expectedType)
			}
			if (err != nil) != tt.expectedError {
				t.Errorf("error: got %v, expected error: %v", err, tt.expectedError)
			}
		})
	}
}

// Helper function to build a PostgreSQL simple query message
// Format: Q + length(4 big-endian) + query_string\0
func buildPgQuery(query string) []byte {
	// Message length includes the 4-byte length field itself + query + null terminator
	msgLen := 4 + len(query) + 1
	data := []byte{'Q'}
	data = append(data, byte(msgLen>>24), byte(msgLen>>16), byte(msgLen>>8), byte(msgLen))
	data = append(data, []byte(query)...)
	data = append(data, 0x00)
	return data
}

func TestNewParser(t *testing.T) {
	tests := []struct {
		protocol string
		expected string
	}{
		{"mysql", "mysql"},
		{"postgresql", "postgresql"},
		{"postgres", "postgresql"},
		{"redis", "redis"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.protocol, func(t *testing.T) {
			parser := NewParser(tt.protocol)
			if parser.Protocol() != tt.expected {
				t.Errorf("got %v, expected %v", parser.Protocol(), tt.expected)
			}
		})
	}
}

func TestQueryTypeIsTxnControl(t *testing.T) {
	tests := []struct {
		queryType QueryType
		expected  bool
	}{
		{QueryTypeBegin, true},
		{QueryTypeCommit, true},
		{QueryTypeRollback, true},
		{QueryTypeSelect, false},
		{QueryTypeInsert, false},
		{QueryTypeUpdate, false},
		{QueryTypeDelete, false},
	}

	for _, tt := range tests {
		t.Run(tt.queryType.String(), func(t *testing.T) {
			result := tt.queryType.IsTxnControl()
			if result != tt.expected {
				t.Errorf("got %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestIsSessionDirtyingStatement(t *testing.T) {
	tests := []struct {
		query    string
		expected bool
	}{
		{"SET SESSION var = value", true},
		{"SET GLOBAL var = value", true},
		{"SET var = value", true},
		{"PREPARE stmt FROM 'SELECT 1'", true},
		{"SELECT * FROM users", false},
		{"INSERT INTO users VALUES (1)", false},
		{"UPDATE users SET name='test'", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			result := IsSessionDirtyingStatement(tt.query)
			if result != tt.expected {
				t.Errorf("got %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestMySQLParserParseWithFullQueries(t *testing.T) {
	parser := &MySQLParser{}

	tests := []struct {
		name        string
		data        []byte
		expectError bool
		expectType  QueryType
	}{
		{
			name:       "SELECT query",
			data:       append([]byte{0x00, 0x00, 0x00, 0x00, 0x03}, []byte("SELECT * FROM users")...),
			expectType: QueryTypeSelect,
		},
		{
			name:       "UPDATE query",
			data:       append([]byte{0x00, 0x00, 0x00, 0x00, 0x03}, []byte("UPDATE users SET name='test'")...),
			expectType: QueryTypeUpdate,
		},
		{
			name:       "DELETE query",
			data:       append([]byte{0x00, 0x00, 0x00, 0x00, 0x03}, []byte("DELETE FROM users WHERE id=1")...),
			expectType: QueryTypeDelete,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.Parse(tt.data)
			if (err != nil) != tt.expectError {
				t.Errorf("unexpected error: %v", err)
			}
			if result != nil && result.QueryType != tt.expectType {
				t.Errorf("expected %v, got %v", tt.expectType, result.QueryType)
			}
		})
	}
}

func TestPostgreSQLParserParseWithFullQueries(t *testing.T) {
	parser := &PostgreSQLParser{}

	tests := []struct {
		name       string
		data       []byte
		expectType QueryType
	}{
		{
			name:       "UPDATE query",
			data:       buildPgQuery("UPDATE users SET name='test'"),
			expectType: QueryTypeUpdate,
		},
		{
			name:       "DELETE query",
			data:       buildPgQuery("DELETE FROM users WHERE id=1"),
			expectType: QueryTypeDelete,
		},
		{
			name:       "BEGIN query",
			data:       buildPgQuery("BEGIN"),
			expectType: QueryTypeBegin,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.Parse(tt.data)
			if err != nil {
				t.Logf("Parse error (may be expected): %v", err)
			}
			if result != nil && result.QueryType != tt.expectType {
				t.Errorf("expected %v, got %v", tt.expectType, result.QueryType)
			}
		})
	}
}
