package security

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

type SQLChecker struct {
	enabled  bool
	patterns []*regexp.Regexp
	redis    *redis.Client
}

type BlockedDatabase struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Pattern string `json:"pattern"`
	Reason  string `json:"reason"`
	Active  bool   `json:"active"`
}

type CheckerThreatIndicator struct {
	ID          int      `json:"id"`
	Type        string   `json:"type"`
	Value       string   `json:"value"`
	ThreatLevel string   `json:"threat_level"`
	Confidence  int      `json:"confidence"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Action      string   `json:"action"`
}

type DatabaseSecurityConfig struct {
	SecurityBlocksEnabled     bool   `json:"security_blocks_enabled"`
	ThreatIntelBlocksEnabled  bool   `json:"threat_intel_blocks_enabled"`
	SQLInjectionDetection     bool   `json:"sql_injection_detection"`
	AuditLogging             bool   `json:"audit_logging"`
	BlockDefaultResources    bool   `json:"block_default_resources"`
	ThreatIntelAction        string `json:"threat_intel_action"`
}

// DefaultBlockedResources contains comprehensive lists of default databases and accounts to block
type DefaultBlockedResources struct {
	Databases []BlockedDatabase `json:"databases"`
	Users     []BlockedDatabase `json:"users"`
	Tables    []BlockedDatabase `json:"tables"`
}

// GetDefaultBlockedResources returns the default blocked resources for all database types
func GetDefaultBlockedResources() DefaultBlockedResources {
	return DefaultBlockedResources{
		Databases: []BlockedDatabase{
			// Common test/demo databases
			{Name: "test", Type: "database", Pattern: "^test$", Reason: "Default test database", Active: true},
			{Name: "sample", Type: "database", Pattern: "^sample$", Reason: "Default sample database", Active: true},
			{Name: "demo", Type: "database", Pattern: "^demo$", Reason: "Default demo database", Active: true},
			{Name: "example", Type: "database", Pattern: "^example$", Reason: "Default example database", Active: true},
			{Name: "temp", Type: "database", Pattern: "^temp$", Reason: "Temporary database", Active: true},
			{Name: "tmp", Type: "database", Pattern: "^tmp$", Reason: "Temporary database", Active: true},
			
			// SQL Server system databases
			{Name: "master", Type: "database", Pattern: "^master$", Reason: "SQL Server system database", Active: true},
			{Name: "msdb", Type: "database", Pattern: "^msdb$", Reason: "SQL Server system database", Active: true},
			{Name: "tempdb", Type: "database", Pattern: "^tempdb$", Reason: "SQL Server system database", Active: true},
			{Name: "model", Type: "database", Pattern: "^model$", Reason: "SQL Server system database", Active: true},
			
			// MySQL system databases
			{Name: "mysql", Type: "database", Pattern: "^mysql$", Reason: "MySQL system database", Active: true},
			{Name: "sys", Type: "database", Pattern: "^sys$", Reason: "MySQL system database", Active: true},
			{Name: "information_schema", Type: "database", Pattern: "^information_schema$", Reason: "MySQL information schema", Active: true},
			{Name: "performance_schema", Type: "database", Pattern: "^performance_schema$", Reason: "MySQL performance schema", Active: true},
			
			// PostgreSQL system databases
			{Name: "postgres", Type: "database", Pattern: "^postgres$", Reason: "PostgreSQL default database", Active: true},
			{Name: "template0", Type: "database", Pattern: "^template0$", Reason: "PostgreSQL template database", Active: true},
			{Name: "template1", Type: "database", Pattern: "^template1$", Reason: "PostgreSQL template database", Active: true},
			
			// MongoDB system databases
			{Name: "admin", Type: "database", Pattern: "^admin$", Reason: "MongoDB admin database", Active: true},
			{Name: "local", Type: "database", Pattern: "^local$", Reason: "MongoDB local database", Active: true},
			{Name: "config", Type: "database", Pattern: "^config$", Reason: "MongoDB config database", Active: true},
			
			// Pattern-based blocking for common naming conventions
			{Name: "test_pattern", Type: "database", Pattern: "^test_.*", Reason: "Test database pattern", Active: true},
			{Name: "sample_pattern", Type: "database", Pattern: "^sample_.*", Reason: "Sample database pattern", Active: true},
			{Name: "demo_pattern", Type: "database", Pattern: "^demo_.*", Reason: "Demo database pattern", Active: true},
			{Name: "backup_pattern", Type: "database", Pattern: ".*_backup$", Reason: "Backup database pattern", Active: true},
			{Name: "old_pattern", Type: "database", Pattern: ".*_old$", Reason: "Old database pattern", Active: true},
		},
		Users: []BlockedDatabase{
			// Common default admin accounts
			{Name: "sa", Type: "username", Pattern: "^sa$", Reason: "SQL Server default admin account", Active: true},
			{Name: "root", Type: "username", Pattern: "^root$", Reason: "Default root account", Active: true},
			{Name: "admin", Type: "username", Pattern: "^admin$", Reason: "Default admin account", Active: true},
			{Name: "administrator", Type: "username", Pattern: "^administrator$", Reason: "Default administrator account", Active: true},
			{Name: "guest", Type: "username", Pattern: "^guest$", Reason: "Default guest account", Active: true},
			
			// Test/demo accounts
			{Name: "test", Type: "username", Pattern: "^test$", Reason: "Test user account", Active: true},
			{Name: "demo", Type: "username", Pattern: "^demo$", Reason: "Demo user account", Active: true},
			{Name: "sample", Type: "username", Pattern: "^sample$", Reason: "Sample user account", Active: true},
			{Name: "user", Type: "username", Pattern: "^user$", Reason: "Generic user account", Active: true},
			
			// Database-specific default accounts
			{Name: "mysql", Type: "username", Pattern: "^mysql$", Reason: "MySQL service account", Active: true},
			{Name: "postgres", Type: "username", Pattern: "^postgres$", Reason: "PostgreSQL default account", Active: true},
			{Name: "oracle", Type: "username", Pattern: "^oracle$", Reason: "Oracle default account", Active: true},
			{Name: "sqlserver", Type: "username", Pattern: "^sqlserver$", Reason: "SQL Server service account", Active: true},
			
			// Empty/blank username
			{Name: "empty", Type: "username", Pattern: "^$", Reason: "Empty/anonymous username", Active: true},
			{Name: "anonymous", Type: "username", Pattern: "^anonymous$", Reason: "Anonymous user account", Active: true},
			
			// Pattern-based blocking
			{Name: "test_pattern", Type: "username", Pattern: "^test_.*", Reason: "Test user pattern", Active: true},
			{Name: "admin_pattern", Type: "username", Pattern: ".*admin.*", Reason: "Admin user pattern", Active: true},
		},
		Tables: []BlockedDatabase{
			// System tables
			{Name: "user", Type: "table", Pattern: "^user$", Reason: "System user table", Active: true},
			{Name: "users", Type: "table", Pattern: "^users$", Reason: "System users table", Active: true},
			{Name: "mysql.user", Type: "table", Pattern: "^mysql\\.user$", Reason: "MySQL user table", Active: true},
			{Name: "pg_user", Type: "table", Pattern: "^pg_user$", Reason: "PostgreSQL user table", Active: true},
			{Name: "sysusers", Type: "table", Pattern: "^sysusers$", Reason: "SQL Server system users", Active: true},
			{Name: "sysobjects", Type: "table", Pattern: "^sysobjects$", Reason: "SQL Server system objects", Active: true},
			{Name: "information_schema", Type: "table", Pattern: "^information_schema\\..*", Reason: "Information schema tables", Active: true},
		},
	}
}

func NewSQLChecker(enabled bool, redisClient *redis.Client) *SQLChecker {
	checker := &SQLChecker{
		enabled: enabled,
		redis:   redisClient,
	}

	if enabled {
		checker.patterns = []*regexp.Regexp{
			// Existing SQL injection patterns
			regexp.MustCompile(`(?i)(\bunion\b.*\bselect\b|\bselect\b.*\bunion\b)`),
			regexp.MustCompile(`(?i)(;\s*drop\s+|;\s*delete\s+|;\s*truncate\s+|;\s*alter\s+)`),
			regexp.MustCompile(`(?i)(\bor\b\s*\d+\s*=\s*\d+|\band\b\s*\d+\s*=\s*\d+)`),
			regexp.MustCompile(`(?i)(--|\#|\/\*|\*\/)`),
			regexp.MustCompile(`(?i)(\bexec\s*\(|\bexecute\s*\()`),
			regexp.MustCompile(`(?i)(\bxp_cmdshell\b|\bcmd\.exe\b)`),
			regexp.MustCompile(`(?i)(\bwaitfor\s+delay\b|\bsleep\s*\()`),
			regexp.MustCompile(`(?i)(\bbenchmark\s*\(|\bpg_sleep\s*\()`),
			regexp.MustCompile(`(?i)(\binformation_schema\b|\bsys\.tables\b|\bsyscolumns\b)`),
			regexp.MustCompile(`(?i)(\bload_file\s*\(|\binto\s+outfile\b|\binto\s+dumpfile\b)`),
			regexp.MustCompile(`(?i)(\bupdatexml\s*\(|\bextractvalue\s*\()`),
			regexp.MustCompile(`(?i)(0x[0-9a-f]+|\bhex\s*\(|\bunhex\s*\()`),
			regexp.MustCompile(`(?i)(\bconcat\s*\(.*\bchar\s*\(|\bchar\s*\(.*\bconcat\s*\()`),
			regexp.MustCompile(`(?i)(\b(having|group\s+by)\b.*\b(select|union)\b)`),
			
			// Enhanced shell command detection patterns
			regexp.MustCompile(`(?i)(\bsystem\s*\(|\bshell_exec\s*\(|\bpassthru\s*\()`),
			regexp.MustCompile(`(?i)(\bproc_open\s*\(|\bpopen\s*\()`),
			regexp.MustCompile(`(?i)(\bcmd\b|\bpowershell\b|\bbash\b|\bsh\b)`),
			regexp.MustCompile(`(?i)(/bin/|/usr/bin/|/sbin/|/usr/sbin/)`),
			regexp.MustCompile(`(?i)(\bchmod\b|\bchown\b|\bsu\b|\bsudo\b)`),
			regexp.MustCompile(`(?i)(\bkill\b|\bkillall\b|\bps\b|\btop\b)`),
			regexp.MustCompile(`(?i)(\bcat\b\s+/|\btail\b\s+/|\bhead\b\s+/)`),
			regexp.MustCompile(`(?i)(\bls\b\s+/|\bfind\b\s+/|\bgrep\b\s+-r)`),
			regexp.MustCompile(`(?i)(\bwget\b|\bcurl\b|\bnc\b|\bnetcat\b)`),
			regexp.MustCompile(`(?i)(\bmkdir\b|\brmdir\b|\brm\b\s+-rf)`),
			regexp.MustCompile(`(?i)(\bcp\b\s+/|\bmv\b\s+/|\btar\b\s+-)`),
			regexp.MustCompile(`(?i)(\bawk\b|\bsed\b|\bperl\b|\bpython\b)`),
			regexp.MustCompile(`(?i)(\bvi\b|\bvim\b|\bnano\b|\bemacs\b)`),
			regexp.MustCompile(`(?i)(\b\|\s*sh\b|\b\|\s*bash\b|\b&&\s*sh\b)`),
			regexp.MustCompile(`(?i)(\$\(.*\)|` + "`" + `.*` + "`" + `|\beval\b)`),
			
			// Additional dangerous system calls
			regexp.MustCompile(`(?i)(\bsp_oacreate\b|\bsp_oamethod\b|\bsp_oadestroy\b)`),
			regexp.MustCompile(`(?i)(\bopenrowset\b|\bopendatasource\b|\blinkedserver\b)`),
			regexp.MustCompile(`(?i)(\bbulk\s+insert\b|\bbcp\b|\bsqlcmd\b)`),
			regexp.MustCompile(`(?i)(\bcreate\s+function\b|\bcreate\s+procedure\b)`),
			regexp.MustCompile(`(?i)(\bgrant\s+execute\b|\bgrant\s+all\b)`),
			
			// Registry and file system access patterns (Windows)
			regexp.MustCompile(`(?i)(\breg\s+add\b|\breg\s+delete\b|\breg\s+query\b)`),
			regexp.MustCompile(`(?i)(\bHKEY_LOCAL_MACHINE\b|\bHKEY_CURRENT_USER\b)`),
			regexp.MustCompile(`(?i)(\bsc\s+create\b|\bsc\s+start\b|\bsc\s+stop\b)`),
			regexp.MustCompile(`(?i)(\bnet\s+user\b|\bnet\s+localgroup\b)`),
			regexp.MustCompile(`(?i)(\bwmic\b|\brundll32\b|\bregsvr32\b)`),
			
			// Default/test database patterns
			regexp.MustCompile(`(?i)(\btest\b|\bsample\b|\bdemo\b|\bexample\b)`),
			regexp.MustCompile(`(?i)(\bmaster\.|\bmsdb\.|\btempdb\.|\bmodel\.)`),
			regexp.MustCompile(`(?i)(\broot\b|\bsa\b|\badmin\b|\bguest\b)`),
			regexp.MustCompile(`(?i)(\bmysql\.user\b|\bmysql\.db\b|\bpg_user\b)`),
		}
	}

	return checker
}

func (c *SQLChecker) IsSQLInjection(query string) bool {
	if !c.enabled {
		return false
	}

	query = strings.ToLower(query)

	for _, pattern := range c.patterns {
		if pattern.MatchString(query) {
			return true
		}
	}

	suspiciousCount := 0
	if strings.Count(query, "'") > 4 {
		suspiciousCount++
	}
	if strings.Count(query, "\"") > 4 {
		suspiciousCount++
	}
	if strings.Contains(query, "1=1") || strings.Contains(query, "1 = 1") {
		suspiciousCount++
	}
	if strings.Contains(query, "' or '") || strings.Contains(query, "\" or \"") {
		suspiciousCount++
	}

	return suspiciousCount >= 2
}

// IsBlockedDatabase checks if the database, table, or username is blocked
func (c *SQLChecker) IsBlockedDatabase(ctx context.Context, database, table, username string) (bool, string) {
	if c.redis == nil {
		return false, ""
	}

	blockedData, err := c.redis.Get(ctx, "articdbm:blocked_databases").Result()
	if err != nil {
		return false, ""
	}

	var blockedDatabases map[string]BlockedDatabase
	if err := json.Unmarshal([]byte(blockedData), &blockedDatabases); err != nil {
		return false, ""
	}

	// Check each blocked resource
	for _, blocked := range blockedDatabases {
		if !blocked.Active {
			continue
		}

		var targetValue string
		switch blocked.Type {
		case "database":
			targetValue = database
		case "table":
			targetValue = table
		case "username":
			targetValue = username
		default:
			continue
		}

		if targetValue == "" {
			continue
		}

		// Check for exact regex pattern match
		matched, err := regexp.MatchString(blocked.Pattern, strings.ToLower(targetValue))
		if err == nil && matched {
			return true, blocked.Reason
		}

		// Fallback to simple string containment for backwards compatibility
		if strings.Contains(strings.ToLower(targetValue), strings.ToLower(blocked.Pattern)) {
			return true, blocked.Reason
		}
	}

	return false, ""
}

// IsBlockedConnection checks all connection parameters for blocked resources
func (c *SQLChecker) IsBlockedConnection(ctx context.Context, database, table, username string) (bool, string) {
	// Check database
	if blocked, reason := c.IsBlockedDatabase(ctx, database, "", ""); blocked {
		return true, reason
	}

	// Check username
	if blocked, reason := c.IsBlockedDatabase(ctx, "", "", username); blocked {
		return true, reason
	}

	// Check table if provided
	if table != "" {
		if blocked, reason := c.IsBlockedDatabase(ctx, "", table, ""); blocked {
			return true, reason
		}
	}

	return false, ""
}

// SeedDefaultBlockedResources initializes Redis with default blocked resources if not present
func (c *SQLChecker) SeedDefaultBlockedResources(ctx context.Context) error {
	if c.redis == nil {
		return nil
	}

	// Check if blocked resources already exist
	exists, err := c.redis.Exists(ctx, "articdbm:blocked_databases").Result()
	if err != nil || exists > 0 {
		return nil // Already seeded or error occurred
	}

	defaultResources := GetDefaultBlockedResources()
	allBlocked := make(map[string]BlockedDatabase)
	
	// Add all default databases
	for i, db := range defaultResources.Databases {
		allBlocked[fmt.Sprintf("db_%d", i)] = db
	}
	
	// Add all default users
	for i, user := range defaultResources.Users {
		allBlocked[fmt.Sprintf("user_%d", i)] = user
	}
	
	// Add all default tables
	for i, table := range defaultResources.Tables {
		allBlocked[fmt.Sprintf("table_%d", i)] = table
	}

	// Store in Redis
	data, err := json.Marshal(allBlocked)
	if err != nil {
		return err
	}

	return c.redis.Set(ctx, "articdbm:blocked_databases", string(data), 0).Err()
}

// IsShellCommand checks specifically for shell command patterns
func (c *SQLChecker) IsShellCommand(query string) bool {
	if !c.enabled {
		return false
	}

	query = strings.ToLower(query)
	
	// Shell command specific patterns
	shellPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(\bsystem\s*\(|\bshell_exec\s*\(|\bpassthru\s*\()`),
		regexp.MustCompile(`(?i)(\bcmd\b|\bpowershell\b|\bbash\b|\bsh\b)`),
		regexp.MustCompile(`(?i)(/bin/|/usr/bin/|/sbin/|/usr/sbin/)`),
		regexp.MustCompile(`(?i)(\bchmod\b|\bchown\b|\bsu\b|\bsudo\b)`),
		regexp.MustCompile(`(?i)(\$\(.*\)|` + "`" + `.*` + "`" + `|\beval\b)`),
		regexp.MustCompile(`(?i)(\b\|\s*sh\b|\b\|\s*bash\b|\b&&\s*sh\b)`),
		regexp.MustCompile(`(?i)(\bxp_cmdshell\b|\bcmd\.exe\b)`),
	}

	for _, pattern := range shellPatterns {
		if pattern.MatchString(query) {
			return true
		}
	}

	return false
}

// Enhanced IsSQLInjection with better categorization
func (c *SQLChecker) IsSQLInjectionWithDetails(query string) (bool, string, string) {
	if !c.enabled {
		return false, "", ""
	}

	query = strings.ToLower(query)

	// Check for shell commands first (critical)
	if c.IsShellCommand(query) {
		return true, "shell_command", "Shell command execution detected"
	}

	// Check regular SQL injection patterns
	for _, pattern := range c.patterns {
		if pattern.MatchString(query) {
			return true, "sql_injection", "SQL injection pattern detected"
		}
	}

	// Enhanced heuristics
	suspiciousCount := 0
	suspiciousReasons := []string{}
	
	if strings.Count(query, "'") > 4 {
		suspiciousCount++
		suspiciousReasons = append(suspiciousReasons, "excessive single quotes")
	}
	if strings.Count(query, "\"") > 4 {
		suspiciousCount++
		suspiciousReasons = append(suspiciousReasons, "excessive double quotes")
	}
	if strings.Contains(query, "1=1") || strings.Contains(query, "1 = 1") {
		suspiciousCount++
		suspiciousReasons = append(suspiciousReasons, "always-true condition")
	}
	if strings.Contains(query, "' or '") || strings.Contains(query, "\" or \"") {
		suspiciousCount++
		suspiciousReasons = append(suspiciousReasons, "quote-based bypass attempt")
	}

	if suspiciousCount >= 2 {
		return true, "heuristic", "Multiple suspicious patterns: " + strings.Join(suspiciousReasons, ", ")
	}

	return false, "", ""
}

// CheckThreatIntel checks query, IP, and other indicators against threat intelligence
func (c *SQLChecker) CheckThreatIntel(ctx context.Context, database string, sourceIP string, query string, username string) (bool, *CheckerThreatIndicator, string) {
	if c.redis == nil {
		return false, nil, ""
	}

	// First check if threat intel blocking is enabled for this database
	configData, err := c.redis.Get(ctx, "articdbm:database_security_configs").Result()
	if err != nil {
		// If no config found, default to enabled
		return c.checkCheckerThreatIndicators(ctx, database, sourceIP, query, username)
	}

	var configs map[string]DatabaseSecurityConfig
	if err := json.Unmarshal([]byte(configData), &configs); err != nil {
		return c.checkCheckerThreatIndicators(ctx, database, sourceIP, query, username)
	}

	// Check if this database has threat intel blocks enabled
	if config, exists := configs[database]; exists {
		if !config.ThreatIntelBlocksEnabled {
			return false, nil, ""
		}
	}

	return c.checkCheckerThreatIndicators(ctx, database, sourceIP, query, username)
}

func (c *SQLChecker) checkCheckerThreatIndicators(ctx context.Context, database string, sourceIP string, query string, username string) (bool, *CheckerThreatIndicator, string) {
	// Get threat indicators from Redis
	indicatorData, err := c.redis.Get(ctx, "articdbm:threat_indicators").Result()
	if err != nil {
		return false, nil, ""
	}

	var indicators map[string]CheckerThreatIndicator
	if err := json.Unmarshal([]byte(indicatorData), &indicators); err != nil {
		return false, nil, ""
	}

	queryLower := strings.ToLower(query)

	// Check various indicator types
	for _, indicator := range indicators {
		matched := false
		matchReason := ""

		switch indicator.Type {
		case "ip":
			if sourceIP == indicator.Value {
				matched = true
				matchReason = fmt.Sprintf("Source IP %s matches threat indicator", sourceIP)
			}
		
		case "sql_pattern":
			if strings.Contains(queryLower, strings.ToLower(indicator.Value)) {
				matched = true
				matchReason = fmt.Sprintf("Query contains threat pattern: %s", indicator.Value)
			}
		
		case "pattern":
			// Generic pattern matching
			if pattern, err := regexp.Compile(indicator.Value); err == nil {
				if pattern.MatchString(queryLower) {
					matched = true
					matchReason = fmt.Sprintf("Query matches threat pattern: %s", indicator.Value)
				}
			}
		
		case "user_agent":
			// This would need to be passed in from the connection context
			// For now, skip user agent checks
			
		case "domain", "url":
			// Check if query contains the indicator value
			if strings.Contains(queryLower, strings.ToLower(indicator.Value)) {
				matched = true
				matchReason = fmt.Sprintf("Query contains threat indicator: %s", indicator.Value)
			}
			
		case "email":
			if strings.EqualFold(username, indicator.Value) {
				matched = true
				matchReason = fmt.Sprintf("Username %s matches threat indicator", username)
			}
		}

		if matched {
			// Record the match
			c.recordThreatMatch(ctx, &indicator, database, username, sourceIP, query, matchReason)
			return true, &indicator, matchReason
		}
	}

	return false, nil, ""
}

func (c *SQLChecker) recordThreatMatch(ctx context.Context, indicator *CheckerThreatIndicator, database, username, sourceIP, query, matchDetails string) {
	// In a production environment, this would write to the database
	// For now, we'll log it to Redis for the manager to pick up
	
	matchData := map[string]interface{}{
		"indicator_id":    indicator.ID,
		"indicator_type":  indicator.Type,
		"indicator_value": indicator.Value,
		"threat_level":   indicator.ThreatLevel,
		"database":       database,
		"username":       username,
		"source_ip":      sourceIP,
		"query":          query,
		"match_details":  matchDetails,
		"timestamp":      fmt.Sprintf("%d", time.Now().Unix()),
		"action_taken":   "blocked",
	}
	
	data, _ := json.Marshal(matchData)
	key := fmt.Sprintf("articdbm:threat_match:%d", time.Now().UnixNano())
	c.redis.Set(ctx, key, string(data), 24*time.Hour)
	
	// Also increment match counter for the indicator
	counterKey := fmt.Sprintf("articdbm:threat_indicator_matches:%d", indicator.ID)
	c.redis.Incr(ctx, counterKey)
}

// GetDatabaseSecurityConfig retrieves security configuration for a specific database
func (c *SQLChecker) GetDatabaseSecurityConfig(ctx context.Context, database string) (*DatabaseSecurityConfig, error) {
	if c.redis == nil {
		return nil, fmt.Errorf("redis client not initialized")
	}

	configData, err := c.redis.Get(ctx, "articdbm:database_security_configs").Result()
	if err != nil {
		// Return default config if not found
		return &DatabaseSecurityConfig{
			SecurityBlocksEnabled:    true,
			ThreatIntelBlocksEnabled: true,
			SQLInjectionDetection:    true,
			AuditLogging:            true,
			BlockDefaultResources:   true,
			ThreatIntelAction:       "block",
		}, nil
	}

	var configs map[string]DatabaseSecurityConfig
	if err := json.Unmarshal([]byte(configData), &configs); err != nil {
		return nil, err
	}

	if config, exists := configs[database]; exists {
		return &config, nil
	}

	// Return default config if database not found
	return &DatabaseSecurityConfig{
		SecurityBlocksEnabled:    true,
		ThreatIntelBlocksEnabled: true,
		SQLInjectionDetection:    true,
		AuditLogging:            true,
		BlockDefaultResources:   true,
		ThreatIntelAction:       "block",
	}, nil
}

func IsWriteQuery(query string) bool {
	query = strings.TrimSpace(strings.ToUpper(query))
	
	writeKeywords := []string{
		"INSERT", "UPDATE", "DELETE", "CREATE", "ALTER", "DROP",
		"TRUNCATE", "RENAME", "REPLACE", "MERGE", "CALL", "LOCK",
		"GRANT", "REVOKE", "SET", "BEGIN", "COMMIT", "ROLLBACK",
		"SAVEPOINT", "RELEASE", "START",
	}

	for _, keyword := range writeKeywords {
		if strings.HasPrefix(query, keyword) {
			return true
		}
	}

	return false
}