package security

import (
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"
)

// Checker implements SQL injection and security threat detection
type Checker struct {
	patterns       []*regexp.Regexp
	blockedCount   atomic.Int64
	inspectedCount atomic.Int64
	logger         *zap.Logger
	mu             sync.RWMutex
}

// NewChecker creates a new security checker
func NewChecker(logger *zap.Logger) *Checker {
	if logger == nil {
		logger = zap.NewNop()
	}

	checker := &Checker{
		logger: logger,
	}

	// Compile common SQL injection patterns
	// These are heuristic-based; production should use more sophisticated parsing
	checker.patterns = []*regexp.Regexp{
		// Union-based injection
		regexp.MustCompile(`(?i)(\bunion\b.*\bselect\b|\bselect\b.*\bfrom\b.*\bunion\b)`),
		// Comment-based injection
		regexp.MustCompile(`(?i)(--|#|/\*|\*/)`),
		// Stacked queries
		regexp.MustCompile(`(?i)(;\s*(drop|delete|update|insert|create|alter|exec|execute))`),
		// Time-based blind injection
		regexp.MustCompile(`(?i)(sleep\s*\(|benchmark\s*\(|waitfor\s*delay)`),
		// Boolean-based injection patterns
		regexp.MustCompile(`(?i)(\bor\b\s+1\s*=\s*1|\band\b\s+1\s*=\s*1)`),
	}

	return checker
}

// CheckQuery inspects a query for potential SQL injection.
// Returns (blocked, reason).
// DEPRECATED: Use CheckParsedQuery instead for protocol-aware checking.
func (c *Checker) CheckQuery(query string) (bool, string) {
	c.inspectedCount.Add(1)

	normalized := strings.TrimSpace(strings.ToLower(query))
	if normalized == "" {
		return false, ""
	}

	// Check against patterns
	for _, pattern := range c.patterns {
		if pattern.MatchString(normalized) {
			c.blockedCount.Add(1)
			reason := "SQL injection pattern detected"
			c.logger.Warn("query blocked",
				zap.String("reason", reason),
				zap.String("pattern", pattern.String()),
			)
			return true, reason
		}
	}

	// Heuristic: excessive SQL keywords
	if c.hasExcessiveSQLKeywords(normalized) {
		c.blockedCount.Add(1)
		reason := "excessive SQL keywords detected"
		c.logger.Warn("query blocked", zap.String("reason", reason))
		return true, reason
	}

	return false, ""
}

// CheckParsedQuery inspects a parsed query for potential SQL injection.
// This is the new path: uses extracted SQL text instead of raw protocol bytes.
// Returns (blocked, reason).
func (c *Checker) CheckParsedQuery(queryText string) (bool, string) {
	c.inspectedCount.Add(1)

	normalized := strings.TrimSpace(strings.ToLower(queryText))
	if normalized == "" {
		return false, ""
	}

	// Check against patterns
	for _, pattern := range c.patterns {
		if pattern.MatchString(normalized) {
			c.blockedCount.Add(1)
			reason := "SQL injection pattern detected"
			c.logger.Warn("query blocked",
				zap.String("reason", reason),
				zap.String("pattern", pattern.String()),
			)
			return true, reason
		}
	}

	// Heuristic: excessive SQL keywords
	if c.hasExcessiveSQLKeywords(normalized) {
		c.blockedCount.Add(1)
		reason := "excessive SQL keywords detected"
		c.logger.Warn("query blocked", zap.String("reason", reason))
		return true, reason
	}

	return false, ""
}

// hasExcessiveSQLKeywords checks for multiple DML statements (stacked queries)
func (c *Checker) hasExcessiveSQLKeywords(query string) bool {
	// Only count DML/DDL keywords that indicate separate statements
	dmlKeywords := []string{
		"select", "insert", "update", "delete",
		"drop", "create", "alter", "truncate",
	}

	count := 0
	for _, keyword := range dmlKeywords {
		// Look for DML keyword boundaries to avoid false positives
		// e.g., "interval" shouldn't match "delete"
		if strings.Contains(" "+query+" ", " "+keyword+" ") {
			count++
		}
	}

	// Flag if 2 or more DML keywords (stacked query pattern)
	return count >= 2
}

// GetStats returns security checker statistics
func (c *Checker) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"inspected_count": c.inspectedCount.Load(),
		"blocked_count":   c.blockedCount.Load(),
		"patterns_loaded": len(c.patterns),
	}
}

// Reset resets the counters
func (c *Checker) Reset() {
	c.blockedCount.Store(0)
	c.inspectedCount.Store(0)
	c.logger.Info("security checker counters reset")
}

// AddPattern adds a custom regex pattern
func (c *Checker) AddPattern(pattern string) error {
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.patterns = append(c.patterns, compiled)
	c.logger.Info("custom pattern added", zap.String("pattern", pattern))
	return nil
}
