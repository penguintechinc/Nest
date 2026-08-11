package security

import (
	"testing"

	"go.uber.org/zap"
)

func TestCheckerMultiplePatterns(t *testing.T) {
	logger := zap.NewNop()
	checker := NewChecker(logger)

	tests := []struct {
		query       string
		shouldBlock bool
		reason      string
	}{
		{"SELECT * FROM users", false, "clean query"},
		{"SELECT * FROM users UNION SELECT 1,2,3", true, "UNION injection"},
		{"SELECT * FROM users; DROP TABLE users;", true, "stacked query"},
		{"SELECT * FROM users WHERE id = 1 -- comment", true, "comment injection"},
		{"SELECT * FROM users WHERE SLEEP(5)", true, "time-based blind"},
		{"SELECT * FROM users WHERE id LIKE 'admin%'", false, "normal LIKE"},
		{"UPDATE users SET active = true", false, "clean UPDATE"},
		{"DELETE FROM users WHERE id > 100", false, "clean DELETE"},
	}

	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			blocked, reason := checker.CheckQuery(tt.query)
			if blocked != tt.shouldBlock {
				t.Errorf("query: %q, blocked=%v (reason: %s), expected %v", tt.query, blocked, reason, tt.shouldBlock)
			}
		})
	}
}

func TestCheckerGetStats(t *testing.T) {
	logger := zap.NewNop()
	checker := NewChecker(logger)

	// Initial stats
	initialStats := checker.GetStats()
	inspected, _ := initialStats["inspected_count"].(int64)
	blocked, _ := initialStats["blocked_count"].(int64)

	if inspected != 0 {
		t.Errorf("expected 0 initial inspections, got %d", inspected)
	}
	if blocked != 0 {
		t.Errorf("expected 0 initial blocks, got %d", blocked)
	}

	// Run some queries
	checker.CheckQuery("SELECT 1")
	checker.CheckQuery("SELECT 1 UNION SELECT 2")

	stats := checker.GetStats()
	inspected, _ = stats["inspected_count"].(int64)
	blocked, _ = stats["blocked_count"].(int64)

	if inspected != 2 {
		t.Errorf("expected 2 inspections, got %d", inspected)
	}
	if blocked != 1 {
		t.Errorf("expected 1 block, got %d", blocked)
	}
}

func TestCheckerAddAndApplyPattern(t *testing.T) {
	logger := zap.NewNop()
	checker := NewChecker(logger)

	// Add a custom pattern
	customPattern := `(?i)custom_block_this`
	err := checker.AddPattern(customPattern)
	if err != nil {
		t.Fatalf("failed to add pattern: %v", err)
	}

	// Test that custom pattern works
	blocked, _ := checker.CheckQuery("CUSTOM_BLOCK_THIS")
	if !blocked {
		t.Error("custom pattern should have blocked the query")
	}

	// Normal queries should still work
	blocked, _ = checker.CheckQuery("SELECT * FROM data")
	if blocked {
		t.Error("normal query should not be blocked")
	}
}

func TestCheckerExcessiveKeywords(t *testing.T) {
	logger := zap.NewNop()
	checker := NewChecker(logger)

	tests := []struct {
		query       string
		shouldBlock bool
	}{
		{"SELECT * FROM users", false},
		{"SELECT id FROM users WHERE active = true", false},
		{"INSERT INTO users VALUES (1)", false},
		{"UPDATE cache SET x = 1", false},
		{"DELETE FROM logs", false},
	}

	for _, tt := range tests {
		blocked := checker.hasExcessiveSQLKeywords(tt.query)
		if blocked != tt.shouldBlock {
			t.Errorf("query: %q, blocked=%v, expected %v", tt.query, blocked, tt.shouldBlock)
		}
	}
}

func TestCheckerConcurrentAccess(t *testing.T) {
	logger := zap.NewNop()
	checker := NewChecker(logger)

	// Concurrent queries
	done := make(chan bool, 50)
	for i := 0; i < 50; i++ {
		go func(idx int) {
			query := "SELECT * FROM users"
			if idx%5 == 0 {
				query = "SELECT * FROM users UNION SELECT 1,2,3"
			}
			checker.CheckQuery(query)
			done <- true
		}(i)
	}

	// Wait for all
	for i := 0; i < 50; i++ {
		<-done
	}

	stats := checker.GetStats()
	inspected, _ := stats["inspected_count"].(int64)

	if inspected != 50 {
		t.Errorf("expected 50 inspections, got %d", inspected)
	}
}

func TestCheckerCheckParsedQuery(t *testing.T) {
	logger := zap.NewNop()
	checker := NewChecker(logger)

	tests := []struct {
		query       string
		shouldBlock bool
		description string
	}{
		{"SELECT * FROM users", false, "clean select"},
		{"SELECT * FROM users UNION SELECT 1,2,3", true, "union injection"},
		{"SELECT * FROM users WHERE id = 1 -- comment", true, "comment injection"},
		{"DELETE FROM users; DROP TABLE users;", true, "stacked query"},
		{"SELECT SLEEP(5) FROM users", true, "time-based injection"},
		{"SELECT * FROM users WHERE active = 1", false, "clean where clause"},
		{"INSERT INTO users VALUES (1, 'test')", false, "clean insert"},
		{"UPDATE users SET active = 1", false, "clean update"},
		{"", false, "empty query"},
		{"   ", false, "whitespace only"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			blocked, reason := checker.CheckParsedQuery(tt.query)
			if blocked != tt.shouldBlock {
				t.Errorf("query: %q, blocked=%v (reason: %s), expected %v", tt.query, blocked, reason, tt.shouldBlock)
			}
		})
	}
}

func TestCheckerCheckParsedQueryBlocking(t *testing.T) {
	logger := zap.NewNop()
	checker := NewChecker(logger)

	query := "SELECT * FROM users UNION SELECT 1,2,3"
	blocked, reason := checker.CheckParsedQuery(query)

	if !blocked {
		t.Error("expected query to be blocked")
	}

	if reason == "" {
		t.Error("expected reason to be set")
	}

	stats := checker.GetStats()
	blocked_count, _ := stats["blocked_count"].(int64)
	if blocked_count != 1 {
		t.Errorf("expected 1 blocked query, got %d", blocked_count)
	}
}

func TestCheckerNewWithNilLogger(t *testing.T) {
	checker := NewChecker(nil)
	if checker == nil {
		t.Error("expected non-nil checker even with nil logger")
	}

	// Should work normally
	blocked, _ := checker.CheckQuery("SELECT 1")
	if blocked {
		t.Error("valid query should not be blocked")
	}
}

func TestCheckerAddPatternError(t *testing.T) {
	logger := zap.NewNop()
	checker := NewChecker(logger)

	// Add an invalid regex pattern
	err := checker.AddPattern("[invalid(regex")
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestCheckerPatternCount(t *testing.T) {
	logger := zap.NewNop()
	checker := NewChecker(logger)

	stats := checker.GetStats()
	patterns, _ := stats["patterns_loaded"].(int)

	if patterns <= 0 {
		t.Errorf("expected at least 1 pattern, got %d", patterns)
	}
}
