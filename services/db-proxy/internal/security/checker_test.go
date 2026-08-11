package security

import (
	"testing"

	"go.uber.org/zap"
)

func TestNewChecker(t *testing.T) {
	logger := zap.NewNop()
	checker := NewChecker(logger)

	if checker == nil {
		t.Error("NewChecker returned nil")
	}

	if len(checker.patterns) == 0 {
		t.Error("patterns not initialized")
	}
}

func TestCheckQueryLegitimate(t *testing.T) {
	logger := zap.NewNop()
	checker := NewChecker(logger)

	tests := []string{
		"SELECT * FROM users",
		"SELECT id, name, email FROM users WHERE id = 1",
		"INSERT INTO users (name, email) VALUES ('John', 'john@example.com')",
		"UPDATE users SET active = true WHERE id = 1",
		"DELETE FROM logs WHERE created_at < NOW() - INTERVAL 30 DAY",
	}

	for _, query := range tests {
		blocked, reason := checker.CheckQuery(query)
		if blocked {
			t.Errorf("legitimate query blocked: %q (reason: %s)", query, reason)
		}
	}

	stats := checker.GetStats()
	inspected, ok := stats["inspected_count"].(int64)
	if !ok || inspected != int64(len(tests)) {
		t.Errorf("expected %d inspections, got %v", len(tests), inspected)
	}

	blocked, ok := stats["blocked_count"].(int64)
	if !ok || blocked != 0 {
		t.Errorf("expected 0 blocks, got %v", blocked)
	}
}

func TestCheckQueryMalicious(t *testing.T) {
	logger := zap.NewNop()
	checker := NewChecker(logger)

	tests := []struct {
		query  string
		reason string
	}{
		{"SELECT * FROM users UNION SELECT 1,2,3", "union injection"},
		{"SELECT * FROM users; DROP TABLE users;", "stacked query"},
		{"SELECT * FROM users WHERE id = 1 -- comment", "comment injection"},
		{"SELECT * FROM users WHERE id = 1 OR 1=1", "boolean injection"},
		{"SELECT * FROM users WHERE id = SLEEP(5)", "time-based blind"},
	}

	for _, tt := range tests {
		blocked, _ := checker.CheckQuery(tt.query)
		if !blocked {
			t.Errorf("%s: query not blocked: %q", tt.reason, tt.query)
		}
	}

	stats := checker.GetStats()
	blocked, ok := stats["blocked_count"].(int64)
	if !ok || blocked != int64(len(tests)) {
		t.Errorf("expected %d blocks, got %v", len(tests), blocked)
	}
}

func TestHasExcessiveSQLKeywords(t *testing.T) {
	logger := zap.NewNop()
	checker := NewChecker(logger)

	tests := []struct {
		query       string
		shouldBlock bool
	}{
		// Normal queries with 1 keyword
		{"select * from users", false},
		{"select * from users where id = 1", false},
		{"insert into users values (1, 'test')", false},

		// Suspicious: 2+ DML keywords (stacked query pattern)
		{"select * from users insert into logs values (1)", true},
		{"select * from users delete from logs update cache set x=1", true},
	}

	for _, tt := range tests {
		blocked := checker.hasExcessiveSQLKeywords(tt.query)
		if blocked != tt.shouldBlock {
			t.Errorf("hasExcessiveSQLKeywords(%q) = %v, expected %v", tt.query, blocked, tt.shouldBlock)
		}
	}
}

func TestCheckerReset(t *testing.T) {
	logger := zap.NewNop()
	checker := NewChecker(logger)

	// Generate some stats
	checker.CheckQuery("SELECT * FROM users")
	checker.CheckQuery("SELECT * FROM users; DROP TABLE users;")

	stats := checker.GetStats()
	if inspected, ok := stats["inspected_count"].(int64); !ok || inspected == 0 {
		t.Error("no inspections recorded")
	}

	if blocked, ok := stats["blocked_count"].(int64); !ok || blocked == 0 {
		t.Error("no blocks recorded")
	}

	// Reset
	checker.Reset()

	stats = checker.GetStats()
	if inspected, ok := stats["inspected_count"].(int64); !ok || inspected != 0 {
		t.Errorf("inspected_count not reset: %v", inspected)
	}

	if blocked, ok := stats["blocked_count"].(int64); !ok || blocked != 0 {
		t.Errorf("blocked_count not reset: %v", blocked)
	}
}

func TestAddPattern(t *testing.T) {
	logger := zap.NewNop()
	checker := NewChecker(logger)

	initialCount := len(checker.patterns)

	err := checker.AddPattern(`(?i)custom_malicious`)
	if err != nil {
		t.Errorf("AddPattern failed: %v", err)
	}

	if len(checker.patterns) != initialCount+1 {
		t.Errorf("pattern not added: expected %d, got %d", initialCount+1, len(checker.patterns))
	}

	// Test that the custom pattern is applied
	blocked, _ := checker.CheckQuery("CUSTOM_MALICIOUS")
	if !blocked {
		t.Error("custom pattern not applied")
	}
}

func TestAddPatternInvalid(t *testing.T) {
	logger := zap.NewNop()
	checker := NewChecker(logger)

	err := checker.AddPattern("(invalid regex")
	if err == nil {
		t.Error("AddPattern should fail with invalid regex")
	}
}

func TestCheckerConcurrency(t *testing.T) {
	logger := zap.NewNop()
	checker := NewChecker(logger)

	// Simulate concurrent queries
	done := make(chan bool, 100)

	for i := 0; i < 100; i++ {
		go func(idx int) {
			query := "SELECT * FROM users"
			if idx%10 == 0 {
				query = "SELECT * FROM users UNION SELECT 1,2,3"
			}
			checker.CheckQuery(query)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	stats := checker.GetStats()
	inspected, ok := stats["inspected_count"].(int64)
	if !ok || inspected != 100 {
		t.Errorf("expected 100 inspections, got %v", inspected)
	}

	blocked, ok := stats["blocked_count"].(int64)
	if !ok || blocked != 10 {
		t.Errorf("expected 10 blocks, got %v", blocked)
	}
}

func BenchmarkCheckQuery(b *testing.B) {
	logger := zap.NewNop()
	checker := NewChecker(logger)

	queries := []string{
		"SELECT * FROM users WHERE id = 1",
		"SELECT * FROM users UNION SELECT 1,2,3",
		"INSERT INTO users (name) VALUES ('test')",
		"UPDATE users SET active = true WHERE id = 1",
		"DELETE FROM logs WHERE created_at < NOW()",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := i % len(queries)
		checker.CheckQuery(queries[idx])
	}
}

func BenchmarkHasExcessiveSQLKeywords(b *testing.B) {
	logger := zap.NewNop()
	checker := NewChecker(logger)

	query := "SELECT * FROM users INSERT INTO logs DELETE FROM cache UPDATE users SET x=1"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.hasExcessiveSQLKeywords(query)
	}
}
