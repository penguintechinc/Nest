package security

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/go-redis/redis/v8"
	"github.com/go-redis/redismock/v8"
	"github.com/stretchr/testify/assert"
)

func TestDefaultBlockedResources(t *testing.T) {
	resources := GetDefaultBlockedResources()
	
	// Test that we have default resources
	assert.Greater(t, len(resources.Databases), 0, "Should have default blocked databases")
	assert.Greater(t, len(resources.Users), 0, "Should have default blocked users")
	assert.Greater(t, len(resources.Tables), 0, "Should have default blocked tables")
	
	// Test specific critical entries
	dbNames := make(map[string]bool)
	for _, db := range resources.Databases {
		dbNames[db.Name] = true
	}
	
	// Check for critical system databases
	assert.True(t, dbNames["master"], "Should block SQL Server master database")
	assert.True(t, dbNames["mysql"], "Should block MySQL system database")
	assert.True(t, dbNames["postgres"], "Should block PostgreSQL default database")
	assert.True(t, dbNames["admin"], "Should block MongoDB admin database")
	
	// Check for test databases
	assert.True(t, dbNames["test"], "Should block test database")
	assert.True(t, dbNames["demo"], "Should block demo database")
	assert.True(t, dbNames["sample"], "Should block sample database")
}

func TestBlockedDatabaseChecking(t *testing.T) {
	db, mock := redismock.NewClientMock()
	ctx := context.Background()
	
	checker := &SQLChecker{
		enabled: true,
		redis:   db,
	}
	
	// Mock blocked databases data
	blockedData := map[string]BlockedDatabase{
		"test_db": {
			Name:    "test",
			Type:    "database", 
			Pattern: "^test$",
			Reason:  "Test database blocked",
			Active:  true,
		},
		"root_user": {
			Name:    "root",
			Type:    "username",
			Pattern: "^root$", 
			Reason:  "Default root account blocked",
			Active:  true,
		},
		"inactive_rule": {
			Name:    "inactive",
			Type:    "database",
			Pattern: "^inactive$",
			Reason:  "Inactive rule",
			Active:  false,
		},
	}
	
	blockedJSON, _ := json.Marshal(blockedData)
	mock.ExpectGet("articdbm:blocked_databases").SetVal(string(blockedJSON))
	
	// Test blocking of exact database name
	blocked, reason := checker.IsBlockedDatabase(ctx, "test", "", "")
	assert.True(t, blocked, "Should block 'test' database")
	assert.Equal(t, "Test database blocked", reason)
	
	// Test blocking of exact username
	mock.ExpectGet("articdbm:blocked_databases").SetVal(string(blockedJSON))
	blocked, reason = checker.IsBlockedDatabase(ctx, "", "", "root")
	assert.True(t, blocked, "Should block 'root' user")
	assert.Equal(t, "Default root account blocked", reason)
	
	// Test inactive rule is ignored
	mock.ExpectGet("articdbm:blocked_databases").SetVal(string(blockedJSON))
	blocked, _ = checker.IsBlockedDatabase(ctx, "inactive", "", "")
	assert.False(t, blocked, "Should not block inactive rules")
	
	// Test non-blocked resource
	mock.ExpectGet("articdbm:blocked_databases").SetVal(string(blockedJSON))
	blocked, _ = checker.IsBlockedDatabase(ctx, "allowed", "", "")
	assert.False(t, blocked, "Should allow non-blocked resources")
	
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBlockedConnectionChecking(t *testing.T) {
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
		"admin_user": {
			Name:    "admin", 
			Type:    "username",
			Pattern: "^admin$",
			Reason:  "Admin user blocked",
			Active:  true,
		},
		"user_table": {
			Name:    "users",
			Type:    "table",
			Pattern: "^users$",
			Reason:  "System users table blocked",
			Active:  true,
		},
	}
	
	blockedJSON, _ := json.Marshal(blockedData)
	
	// Test database blocking
	mock.ExpectGet("articdbm:blocked_databases").SetVal(string(blockedJSON))
	blocked, reason := checker.IsBlockedConnection(ctx, "test", "", "normaluser")
	assert.True(t, blocked, "Should block connection to blocked database")
	assert.Equal(t, "Test database blocked", reason)
	
	// Test user blocking  
	mock.ExpectGet("articdbm:blocked_databases").SetVal(string(blockedJSON))
	mock.ExpectGet("articdbm:blocked_databases").SetVal(string(blockedJSON))
	blocked, reason = checker.IsBlockedConnection(ctx, "normaldb", "", "admin")
	assert.True(t, blocked, "Should block connection with blocked user")
	assert.Equal(t, "Admin user blocked", reason)
	
	// Test table blocking
	mock.ExpectGet("articdbm:blocked_databases").SetVal(string(blockedJSON))
	mock.ExpectGet("articdbm:blocked_databases").SetVal(string(blockedJSON))
	mock.ExpectGet("articdbm:blocked_databases").SetVal(string(blockedJSON))
	blocked, reason = checker.IsBlockedConnection(ctx, "normaldb", "users", "normaluser") 
	assert.True(t, blocked, "Should block access to blocked table")
	assert.Equal(t, "System users table blocked", reason)
	
	// Test allowed connection
	mock.ExpectGet("articdbm:blocked_databases").SetVal(string(blockedJSON))
	mock.ExpectGet("articdbm:blocked_databases").SetVal(string(blockedJSON))
	mock.ExpectGet("articdbm:blocked_databases").SetVal(string(blockedJSON))
	blocked, _ = checker.IsBlockedConnection(ctx, "alloweddb", "allowedtable", "alloweduser")
	assert.False(t, blocked, "Should allow connection with no blocked resources")
	
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPatternMatching(t *testing.T) {
	db, mock := redismock.NewClientMock()
	ctx := context.Background()
	
	checker := &SQLChecker{
		enabled: true,
		redis:   db,
	}
	
	blockedData := map[string]BlockedDatabase{
		"test_pattern": {
			Name:    "test_pattern", 
			Type:    "database",
			Pattern: "^test_.*",
			Reason:  "Test database pattern",
			Active:  true,
		},
		"admin_pattern": {
			Name:    "admin_pattern",
			Type:    "username", 
			Pattern: ".*admin.*",
			Reason:  "Admin user pattern",
			Active:  true,
		},
	}
	
	blockedJSON, _ := json.Marshal(blockedData)
	
	// Test regex pattern matching for database
	mock.ExpectGet("articdbm:blocked_databases").SetVal(string(blockedJSON))
	blocked, reason := checker.IsBlockedDatabase(ctx, "test_development", "", "")
	assert.True(t, blocked, "Should match test_* pattern")
	assert.Equal(t, "Test database pattern", reason)
	
	// Test regex pattern matching for username
	mock.ExpectGet("articdbm:blocked_databases").SetVal(string(blockedJSON))
	blocked, reason = checker.IsBlockedDatabase(ctx, "", "", "superadmin")
	assert.True(t, blocked, "Should match *admin* pattern")
	assert.Equal(t, "Admin user pattern", reason)
	
	// Test non-matching patterns
	mock.ExpectGet("articdbm:blocked_databases").SetVal(string(blockedJSON))
	blocked, _ = checker.IsBlockedDatabase(ctx, "production", "", "")
	assert.False(t, blocked, "Should not match non-test database")
	
	mock.ExpectGet("articdbm:blocked_databases").SetVal(string(blockedJSON))
	blocked, _ = checker.IsBlockedDatabase(ctx, "", "", "normaluser")
	assert.False(t, blocked, "Should not match non-admin user")
	
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSeedDefaultBlockedResources(t *testing.T) {
	db, mock := redismock.NewClientMock()
	ctx := context.Background()
	
	checker := &SQLChecker{
		enabled: true,
		redis:   db,
	}
	
	// Mock that no existing resources exist
	mock.ExpectExists("articdbm:blocked_databases").SetVal(0)
	
	// Mock the SET operation for seeding
	mock.ExpectSet("articdbm:blocked_databases", mock.AnyValue(), 0).SetVal("OK")
	
	err := checker.SeedDefaultBlockedResources(ctx)
	assert.NoError(t, err, "Should successfully seed default resources")
	
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSeedDefaultBlockedResourcesAlreadyExists(t *testing.T) {
	db, mock := redismock.NewClientMock()
	ctx := context.Background()
	
	checker := &SQLChecker{
		enabled: true,
		redis:   db,
	}
	
	// Mock that resources already exist
	mock.ExpectExists("articdbm:blocked_databases").SetVal(1)
	
	err := checker.SeedDefaultBlockedResources(ctx)
	assert.NoError(t, err, "Should not error when resources already exist")
	
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDatabaseTypeSpecificBlocking(t *testing.T) {
	resources := GetDefaultBlockedResources()
	
	// Check SQL Server specific databases
	sqlServerDbs := []string{"master", "msdb", "tempdb", "model"}
	for _, dbName := range sqlServerDbs {
		found := false
		for _, resource := range resources.Databases {
			if resource.Name == dbName {
				found = true
				assert.Contains(t, resource.Reason, "SQL Server", "SQL Server database should mention SQL Server in reason")
				break
			}
		}
		assert.True(t, found, "Should have SQL Server database: %s", dbName)
	}
	
	// Check MySQL specific databases  
	mysqlDbs := []string{"mysql", "sys", "information_schema", "performance_schema"}
	for _, dbName := range mysqlDbs {
		found := false
		for _, resource := range resources.Databases {
			if resource.Name == dbName {
				found = true
				assert.Contains(t, resource.Reason, "MySQL", "MySQL database should mention MySQL in reason")
				break
			}
		}
		assert.True(t, found, "Should have MySQL database: %s", dbName)
	}
	
	// Check PostgreSQL specific databases
	postgresDbs := []string{"postgres", "template0", "template1"}
	for _, dbName := range postgresDbs {
		found := false
		for _, resource := range resources.Databases {
			if resource.Name == dbName {
				found = true
				assert.Contains(t, resource.Reason, "PostgreSQL", "PostgreSQL database should mention PostgreSQL in reason")
				break
			}
		}
		assert.True(t, found, "Should have PostgreSQL database: %s", dbName)
	}
	
	// Check MongoDB specific databases
	mongoDbs := []string{"admin", "local", "config"}
	for _, dbName := range mongoDbs {
		found := false
		for _, resource := range resources.Databases {
			if resource.Name == dbName {
				found = true
				assert.Contains(t, resource.Reason, "MongoDB", "MongoDB database should mention MongoDB in reason")
				break
			}
		}
		assert.True(t, found, "Should have MongoDB database: %s", dbName)
	}
}

func TestCriticalAccountBlocking(t *testing.T) {
	resources := GetDefaultBlockedResources()
	
	criticalAccounts := []string{"sa", "root", "admin", "administrator", "guest"}
	
	for _, accountName := range criticalAccounts {
		found := false
		for _, resource := range resources.Users {
			if resource.Name == accountName {
				found = true
				assert.Equal(t, "username", resource.Type, "Should be username type")
				assert.True(t, resource.Active, "Critical accounts should be active by default")
				break
			}
		}
		assert.True(t, found, "Should have critical account blocked: %s", accountName)
	}
	
	// Test that test accounts are also blocked
	testAccounts := []string{"test", "demo", "sample"}
	for _, accountName := range testAccounts {
		found := false
		for _, resource := range resources.Users {
			if resource.Name == accountName {
				found = true
				assert.Contains(t, resource.Reason, "account", "Test account should mention account in reason")
				break
			}
		}
		assert.True(t, found, "Should have test account blocked: %s", accountName)
	}
}