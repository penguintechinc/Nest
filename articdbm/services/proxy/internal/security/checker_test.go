package security

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/go-redis/redismock/v8"
)

func TestSQLChecker_IsSQLInjection(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected bool
	}{
		{
			name:     "Clean SELECT query",
			query:    "SELECT id, name FROM users WHERE active = 1",
			expected: false,
		},
		{
			name:     "Union-based injection",
			query:    "SELECT * FROM users WHERE id = 1 UNION SELECT password FROM admin",
			expected: true,
		},
		{
			name:     "OR 1=1 injection",
			query:    "SELECT * FROM users WHERE id = 1 OR 1=1",
			expected: true,
		},
		{
			name:     "Comment-based injection",
			query:    "SELECT * FROM users WHERE id = 1 -- AND password = 'secret'",
			expected: true,
		},
		{
			name:     "Excessive quotes heuristic",
			query:    "SELECT * FROM users WHERE name = '''' OR '''' = ''''",
			expected: true,
		},
	}

	checker := NewSQLChecker(true, nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checker.IsSQLInjection(tt.query)
			if result != tt.expected {
				t.Errorf("IsSQLInjection() = %v, expected %v for query: %s", result, tt.expected, tt.query)
			}
		})
	}
}

func TestSQLChecker_IsShellCommand(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected bool
	}{
		{
			name:     "Clean SQL query",
			query:    "SELECT * FROM users",
			expected: false,
		},
		{
			name:     "xp_cmdshell command",
			query:    "SELECT * FROM users; EXEC xp_cmdshell 'whoami'",
			expected: true,
		},
		{
			name:     "System call attempt",
			query:    "SELECT system('ls -la')",
			expected: true,
		},
		{
			name:     "Bash command injection",
			query:    "SELECT * FROM users WHERE name = 'test'; bash -c 'rm -rf /'",
			expected: true,
		},
		{
			name:     "PowerShell execution",
			query:    "SELECT * FROM users; powershell Get-Process",
			expected: true,
		},
		{
			name:     "Unix binary path",
			query:    "SELECT * FROM users WHERE cmd = '/bin/sh'",
			expected: true,
		},
		{
			name:     "Command chaining",
			query:    "SELECT * FROM users | sh",
			expected: true,
		},
	}

	checker := NewSQLChecker(true, nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checker.IsShellCommand(tt.query)
			if result != tt.expected {
				t.Errorf("IsShellCommand() = %v, expected %v for query: %s", result, tt.expected, tt.query)
			}
		})
	}
}

func TestSQLChecker_IsSQLInjectionWithDetails(t *testing.T) {
	tests := []struct {
		name            string
		query           string
		expectedBlocked bool
		expectedType    string
	}{
		{
			name:            "Clean query",
			query:           "SELECT * FROM users WHERE active = 1",
			expectedBlocked: false,
			expectedType:    "",
		},
		{
			name:            "Shell command detection",
			query:           "SELECT * FROM users; xp_cmdshell 'dir'",
			expectedBlocked: true,
			expectedType:    "shell_command",
		},
		{
			name:            "SQL injection detection",
			query:           "SELECT * FROM users WHERE id = 1 OR 1=1",
			expectedBlocked: true,
			expectedType:    "sql_injection",
		},
		{
			name:            "Heuristic detection",
			query:           "SELECT * FROM users WHERE name = '''''''' AND description = \"\"\"\"\"\"\"\"",
			expectedBlocked: true,
			expectedType:    "heuristic",
		},
	}

	checker := NewSQLChecker(true, nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocked, attackType, _ := checker.IsSQLInjectionWithDetails(tt.query)
			if blocked != tt.expectedBlocked {
				t.Errorf("IsSQLInjectionWithDetails() blocked = %v, expected %v", blocked, tt.expectedBlocked)
			}
			if attackType != tt.expectedType {
				t.Errorf("IsSQLInjectionWithDetails() type = %v, expected %v", attackType, tt.expectedType)
			}
		})
	}
}

func TestSQLChecker_IsBlockedDatabase(t *testing.T) {
	// Create a mock Redis client
	db, mock := redismock.NewClientMock()

	// Mock blocked databases data
	blockedDatabases := map[string]BlockedDatabase{
		"1": {
			Name:    "block_test_dbs",
			Type:    "database",
			Pattern: "test",
			Reason:  "Test databases not allowed in production",
			Active:  true,
		},
		"2": {
			Name:    "block_admin_users",
			Type:    "username",
			Pattern: "admin",
			Reason:  "Admin users should use dedicated connections",
			Active:  true,
		},
		"3": {
			Name:    "block_sensitive_tables",
			Type:    "table",
			Pattern: "password",
			Reason:  "Password tables require special access",
			Active:  true,
		},
	}

	blockedJSON, _ := json.Marshal(blockedDatabases)
	mock.ExpectGet("articdbm:blocked_databases").SetVal(string(blockedJSON))

	checker := NewSQLChecker(true, db)
	ctx := context.Background()

	tests := []struct {
		name             string
		database         string
		table            string
		username         string
		expectedBlocked  bool
		expectedContains string
	}{
		{
			name:             "Blocked test database",
			database:         "test_db",
			table:            "users",
			username:         "regular_user",
			expectedBlocked:  true,
			expectedContains: "Test databases",
		},
		{
			name:             "Blocked admin user",
			database:         "production",
			table:            "users",
			username:         "admin",
			expectedBlocked:  true,
			expectedContains: "Admin users",
		},
		{
			name:             "Blocked password table",
			database:         "production",
			table:            "password",
			username:         "regular_user",
			expectedBlocked:  true,
			expectedContains: "Password tables",
		},
		{
			name:            "Allowed access",
			database:        "production",
			table:           "users",
			username:        "regular_user",
			expectedBlocked: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocked, reason := checker.IsBlockedDatabase(ctx, tt.database, tt.table, tt.username)
			if blocked != tt.expectedBlocked {
				t.Errorf("IsBlockedDatabase() = %v, expected %v", blocked, tt.expectedBlocked)
			}
			if tt.expectedBlocked && tt.expectedContains != "" {
				if len(reason) == 0 {
					t.Errorf("Expected reason to contain '%s', but got empty reason", tt.expectedContains)
				}
			}
		})
	}

	// Verify all mock expectations were met
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Redis mock expectations were not met: %v", err)
	}
}

func TestIsWriteQuery(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected bool
	}{
		{
			name:     "SELECT query (read)",
			query:    "SELECT * FROM users",
			expected: false,
		},
		{
			name:     "INSERT query (write)",
			query:    "INSERT INTO users (name) VALUES ('John')",
			expected: true,
		},
		{
			name:     "UPDATE query (write)",
			query:    "UPDATE users SET name = 'Jane' WHERE id = 1",
			expected: true,
		},
		{
			name:     "DELETE query (write)",
			query:    "DELETE FROM users WHERE id = 1",
			expected: true,
		},
		{
			name:     "CREATE query (write)",
			query:    "CREATE TABLE test (id INT)",
			expected: true,
		},
		{
			name:     "DROP query (write)",
			query:    "DROP TABLE test",
			expected: true,
		},
		{
			name:     "TRUNCATE query (write)",
			query:    "TRUNCATE TABLE users",
			expected: true,
		},
		{
			name:     "GRANT query (write)",
			query:    "GRANT SELECT ON users TO user1",
			expected: true,
		},
		{
			name:     "Lowercase query",
			query:    "insert into users values (1, 'test')",
			expected: true,
		},
		{
			name:     "Query with whitespace",
			query:    "   UPDATE users SET active = 1   ",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsWriteQuery(tt.query)
			if result != tt.expected {
				t.Errorf("IsWriteQuery() = %v, expected %v for query: %s", result, tt.expected, tt.query)
			}
		})
	}
}

func TestSQLChecker_DisabledChecker(t *testing.T) {
	checker := NewSQLChecker(false, nil)

	// All security checks should return false when checker is disabled
	tests := []string{
		"SELECT * FROM users WHERE id = 1 OR 1=1",
		"SELECT * FROM users; xp_cmdshell 'whoami'",
		"SELECT * FROM users UNION SELECT password FROM admin",
	}

	for _, query := range tests {
		if checker.IsSQLInjection(query) {
			t.Errorf("Disabled checker should not detect injection for: %s", query)
		}
		if checker.IsShellCommand(query) {
			t.Errorf("Disabled checker should not detect shell command for: %s", query)
		}
		if blocked, _, _ := checker.IsSQLInjectionWithDetails(query); blocked {
			t.Errorf("Disabled checker should not block query: %s", query)
		}
	}
}

func TestAdvancedSecurityPatterns(t *testing.T) {
	checker := NewSQLChecker(true, nil)

	tests := []struct {
		name        string
		query       string
		shouldBlock bool
		description string
	}{
		{
			name:        "Registry access attempt",
			query:       "SELECT * FROM users; EXEC xp_regread 'HKEY_LOCAL_MACHINE'",
			shouldBlock: true,
			description: "Registry access should be blocked",
		},
		{
			name:        "Bulk insert operation",
			query:       "BULK INSERT users FROM 'c:\\temp\\users.txt'",
			shouldBlock: true,
			description: "Bulk insert operations should be detected",
		},
		{
			name:        "File load operation",
			query:       "SELECT LOAD_FILE('/etc/passwd')",
			shouldBlock: true,
			description: "File load operations should be blocked",
		},
		{
			name:        "System information disclosure",
			query:       "SELECT @@version",
			shouldBlock: true,
			description: "System information queries should be detected",
		},
		{
			name:        "Information schema access",
			query:       "SELECT * FROM information_schema.tables",
			shouldBlock: true,
			description: "Information schema access should be controlled",
		},
		{
			name:        "Master database access",
			query:       "SELECT * FROM master.dbo.sysdatabases",
			shouldBlock: true,
			description: "Master database access should be blocked",
		},
		{
			name:        "Hex encoding attempt",
			query:       "SELECT * FROM users WHERE name = 0x41646d696e",
			shouldBlock: true,
			description: "Hex-encoded values should be detected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checker.IsSQLInjection(tt.query)
			if result != tt.shouldBlock {
				t.Errorf("%s: IsSQLInjection() = %v, expected %v for query: %s", 
					tt.description, result, tt.shouldBlock, tt.query)
			}
		})
	}
}

func BenchmarkSQLChecker_IsSQLInjection(b *testing.B) {
	checker := NewSQLChecker(true, nil)
	query := "SELECT * FROM users WHERE id = 1 AND name = 'test' OR 1=1"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.IsSQLInjection(query)
	}
}

func BenchmarkSQLChecker_IsShellCommand(b *testing.B) {
	checker := NewSQLChecker(true, nil)
	query := "SELECT * FROM users; xp_cmdshell 'whoami'"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.IsShellCommand(query)
	}
}