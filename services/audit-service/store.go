package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// AuditEventRecord is the persistent database model for audit events.
type AuditEventRecord struct {
	Seq        int64     `gorm:"autoIncrement;primaryKey;column:seq" json:"seq"`
	ID         string    `gorm:"index;column:id" json:"id"`
	Tenant     string    `gorm:"index;column:tenant" json:"tenant"`
	UserUUID   string    `gorm:"index;column:user_uuid" json:"user_uuid"` // UUID only, never PII
	Action     string    `gorm:"column:action" json:"action"`
	Resource   string    `gorm:"column:resource" json:"resource"`
	ResourceID string    `gorm:"column:resource_id" json:"resource_id"`
	Outcome    string    `gorm:"column:outcome" json:"outcome"`     // success, denied, error - CRITICAL for audit trail
	SourceIP   string    `gorm:"column:source_ip" json:"source_ip"` // Client IP - part of audit trail
	Timestamp  time.Time `gorm:"index;column:timestamp" json:"timestamp"`
	Metadata   string    `gorm:"type:text;column:metadata" json:"metadata"` // JSON serialized
	PrevHash   string    `gorm:"column:prev_hash" json:"prev_hash"`
	Hash       string    `gorm:"index;column:hash" json:"hash"`
}

// TableName sets the table name for AuditEventRecord.
func (AuditEventRecord) TableName() string {
	return "audit_events"
}

// DurableStore is the persistent, tamper-evident audit event store.
type DurableStore struct {
	db     *gorm.DB
	mu     sync.Mutex
	logger *zap.Logger
}

// NewDurableStore creates a new DurableStore backed by a database.
// It reads configuration from environment variables:
// - DB_TYPE: postgresql, mysql, or sqlite (default: sqlite)
// - DB_HOST, DB_PORT, DB_NAME, DB_USER, DB_PASS (for postgresql/mysql)
// - DB_POOL_SIZE: connection pool size (default: 10)
// - DB_MAX_RETRIES: number of retry attempts (default: 5)
// - DB_RETRY_DELAY: initial retry delay in seconds (default: 1)
func NewDurableStore(logger *zap.Logger) (*DurableStore, error) {
	dbType := os.Getenv("DB_TYPE")
	if dbType == "" {
		dbType = "sqlite"
	}

	var db *gorm.DB
	var err error

	switch dbType {
	case "postgresql":
		db, err = initPostgres(logger)
	case "mysql":
		db, err = initMySQL(logger)
	case "sqlite":
		db, err = initSQLite(logger)
	default:
		return nil, fmt.Errorf("unsupported DB_TYPE: %s", dbType)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Run migrations
	if err := db.AutoMigrate(&AuditEventRecord{}); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return &DurableStore{
		db:     db,
		logger: logger,
	}, nil
}

// initPostgres initializes a PostgreSQL connection with retry logic.
func initPostgres(logger *zap.Logger) (*gorm.DB, error) {
	host := os.Getenv("DB_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("DB_PORT")
	if port == "" {
		port = "5432"
	}
	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "audit"
	}
	user := os.Getenv("DB_USER")
	if user == "" {
		user = "postgres"
	}
	pass := os.Getenv("DB_PASS")

	// SECURITY: enforce TLS, only allow disable for localhost
	sslmode := os.Getenv("DB_SSLMODE")
	if sslmode == "" {
		if host == "localhost" || host == "127.0.0.1" {
			sslmode = "disable" // Only OK for local development
		} else {
			sslmode = "require" // Enforce TLS for remote connections
		}
	}

	// SECURITY HARDENING: prevent operator from accidentally disabling TLS on remote hosts
	isLocalhost := host == "localhost" || host == "127.0.0.1"
	if !isLocalhost && sslmode == "disable" {
		logger.Warn("TLS disabled on remote PostgreSQL host — forcing to require",
			zap.String("host", host), zap.String("requested_sslmode", sslmode))
		sslmode = "require"
	}

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, pass, dbName, sslmode)

	maxRetries := getEnvInt("DB_MAX_RETRIES", 5)
	retryDelay := getEnvInt("DB_RETRY_DELAY", 1)

	var db *gorm.DB
	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			break
		}
		if attempt < maxRetries-1 {
			delay := time.Duration(retryDelay*(1<<uint(attempt))) * time.Second
			logger.Warn("postgres connection failed, retrying",
				zap.Int("attempt", attempt+1),
				zap.Int("max_retries", maxRetries),
				zap.Duration("retry_delay", delay),
				zap.Error(err))
			time.Sleep(delay)
		}
	}
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	poolSize := getEnvInt("DB_POOL_SIZE", 10)
	sqlDB.SetMaxOpenConns(poolSize)
	sqlDB.SetMaxIdleConns(poolSize / 2)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return db, nil
}

// initMySQL initializes a MySQL connection with retry logic.
func initMySQL(logger *zap.Logger) (*gorm.DB, error) {
	host := os.Getenv("DB_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("DB_PORT")
	if port == "" {
		port = "3306"
	}
	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "audit"
	}
	user := os.Getenv("DB_USER")
	if user == "" {
		user = "root"
	}
	pass := os.Getenv("DB_PASS")

	// SECURITY: enforce TLS, only allow skip for localhost
	tlsMode := "true" // default: require TLS
	isLocalhost := host == "localhost" || host == "127.0.0.1"
	if isLocalhost {
		tlsMode = os.Getenv("DB_TLS")
		if tlsMode == "" {
			tlsMode = "false" // Allow plaintext for local dev only
		}
	}

	// SECURITY HARDENING: prevent operator from accidentally disabling TLS on remote hosts
	if !isLocalhost && (tlsMode == "false" || tlsMode == "skip-verify") {
		logger.Warn("TLS disabled on remote MySQL host — forcing to true",
			zap.String("host", host), zap.String("requested_tls", tlsMode))
		tlsMode = "true"
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local&tls=%s",
		user, pass, host, port, dbName, tlsMode)

	maxRetries := getEnvInt("DB_MAX_RETRIES", 5)
	retryDelay := getEnvInt("DB_RETRY_DELAY", 1)

	var db *gorm.DB
	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
		if err == nil {
			break
		}
		if attempt < maxRetries-1 {
			delay := time.Duration(retryDelay*(1<<uint(attempt))) * time.Second
			logger.Warn("mysql connection failed, retrying",
				zap.Int("attempt", attempt+1),
				zap.Int("max_retries", maxRetries),
				zap.Duration("retry_delay", delay),
				zap.Error(err))
			time.Sleep(delay)
		}
	}
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	poolSize := getEnvInt("DB_POOL_SIZE", 10)
	sqlDB.SetMaxOpenConns(poolSize)
	sqlDB.SetMaxIdleConns(poolSize / 2)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return db, nil
}

// initSQLite initializes a SQLite connection.
func initSQLite(logger *zap.Logger) (*gorm.DB, error) {
	dbPath := os.Getenv("DB_NAME")
	if dbPath == "" {
		dbPath = "audit.db"
	}

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxOpenConns(5)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return db, nil
}

// getEnvInt retrieves an integer environment variable with a default.
func getEnvInt(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	var i int
	if _, err := fmt.Sscanf(val, "%d", &i); err != nil {
		return defaultVal
	}
	return i
}

// computeHash computes the SHA256 hash of an audit event record.
// CRITICAL: includes all audit-trail fields (outcome, source_ip) to prevent tampering.
func computeHash(seq int64, id, tenant, userUUID, action, resource, resourceID, outcome, sourceIP string,
	timestamp time.Time, metadata, prevHash string) string {
	// Canonical form: seq|id|tenant|user_uuid|action|resource|resource_id|outcome|source_ip|timestamp|metadata|prev_hash
	// All fields must be present to detect tampering (e.g., success→denied alteration)
	canonical := fmt.Sprintf("%d|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
		seq, id, tenant, userUUID, action, resource, resourceID, outcome, sourceIP,
		timestamp.UTC().Format(time.RFC3339Nano), metadata, prevHash)
	hash := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(hash[:])
}

// Append adds an event to the durable store and returns the stored record with hash.
// Returns error on validation failure or database error.
func (ds *DurableStore) Append(event *AuditEvent) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	// Validation: required fields
	if event.Tenant == "" {
		return fmt.Errorf("tenant is required")
	}
	if event.Actor == "" {
		return fmt.Errorf("actor is required")
	}
	if event.Action == "" {
		return fmt.Errorf("action is required")
	}
	if event.Resource == "" {
		return fmt.Errorf("resource is required")
	}
	if event.Outcome == "" {
		return fmt.Errorf("outcome is required")
	}

	// Generate ID if not provided
	if event.ID == "" {
		event.ID = fmt.Sprintf("evt-%d", time.Now().UnixNano())
	}

	// Set timestamp if not provided
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Convert details to JSON string
	var metadataJSON string
	if event.Details != nil {
		data, err := json.Marshal(event.Details)
		if err != nil {
			return fmt.Errorf("failed to marshal details: %w", err)
		}
		metadataJSON = string(data)
	}

	// Actor is the user_uuid (already validated as non-empty)
	userUUID := event.Actor

	// Get the previous hash (last event in the chain)
	var lastRecord AuditEventRecord
	ds.db.Order("seq DESC").Limit(1).First(&lastRecord)
	prevHash := lastRecord.Hash

	// Create the record without hash first
	record := AuditEventRecord{
		ID:         event.ID,
		Tenant:     event.Tenant,
		UserUUID:   userUUID,
		Action:     event.Action,
		Resource:   event.Resource,
		ResourceID: "",
		Outcome:    event.Outcome,  // CRITICAL: must be stored and included in hash
		SourceIP:   event.SourceIP, // Part of audit trail
		Timestamp:  event.Timestamp,
		Metadata:   metadataJSON,
		PrevHash:   prevHash,
		Hash:       "",
	}

	// Insert the record without hash to get the seq
	if err := ds.db.Create(&record).Error; err != nil {
		return fmt.Errorf("failed to insert record: %w", err)
	}

	// Now compute the hash with the actual seq (includes outcome and source_ip for tamper-detection)
	hash := computeHash(record.Seq, record.ID, record.Tenant, record.UserUUID,
		record.Action, record.Resource, record.ResourceID, record.Outcome, record.SourceIP,
		record.Timestamp, record.Metadata, record.PrevHash)

	// Update the record with the computed hash
	if err := ds.db.Model(&record).Update("hash", hash).Error; err != nil {
		return fmt.Errorf("failed to update hash: %w", err)
	}

	ds.logger.Debug("audit event appended",
		zap.String("id", event.ID),
		zap.String("action", event.Action),
		zap.String("tenant", event.Tenant),
		zap.Int64("seq", record.Seq))

	return nil
}

// Query returns events matching the filter criteria from the durable store.
func (ds *DurableStore) Query(filter AuditFilter) []*AuditEvent {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	// Set default/max limit
	limit := filter.Limit
	if limit <= 0 {
		limit = 100 // Default limit
	} else if limit > 1000 {
		limit = 1000 // Cap max limit at 1000 to prevent runaway queries
	}

	// Build the query
	query := ds.db.Order("seq ASC")

	// Apply filters
	if filter.Tenant != "" {
		query = query.Where("tenant = ?", filter.Tenant)
	}
	if filter.Actor != "" {
		query = query.Where("user_uuid = ?", filter.Actor)
	}
	if filter.Action != "" {
		query = query.Where("action = ?", filter.Action)
	}
	if filter.Resource != "" {
		query = query.Where("resource = ?", filter.Resource)
	}
	if filter.Outcome != "" {
		query = query.Where("outcome = ?", filter.Outcome)
	}
	if !filter.StartTime.IsZero() {
		query = query.Where("timestamp >= ?", filter.StartTime)
	}
	if !filter.EndTime.IsZero() {
		query = query.Where("timestamp <= ?", filter.EndTime)
	}

	// Apply offset and limit
	query = query.Offset(filter.Offset).Limit(int(limit))

	// Execute query
	var records []AuditEventRecord
	if err := query.Find(&records).Error; err != nil {
		ds.logger.Error("failed to query events", zap.Error(err))
		return []*AuditEvent{}
	}

	// Convert records to AuditEvent
	events := make([]*AuditEvent, 0)
	for _, record := range records {
		var details map[string]interface{}
		if record.Metadata != "" {
			if err := json.Unmarshal([]byte(record.Metadata), &details); err != nil {
				ds.logger.Warn("failed to unmarshal metadata", zap.Error(err))
			}
		}

		event := &AuditEvent{
			ID:        record.ID,
			Tenant:    record.Tenant,
			Actor:     record.UserUUID,
			Action:    record.Action,
			Resource:  record.Resource,
			Outcome:   record.Outcome,  // Restore outcome - part of audit trail
			SourceIP:  record.SourceIP, // Restore source IP
			Timestamp: record.Timestamp,
			Details:   details,
		}
		events = append(events, event)
	}

	return events
}

// ValidateIntegrity walks the hash chain and detects any tampering.
// Returns nil if the chain is valid, or an error describing the first break found.
func (ds *DurableStore) ValidateIntegrity(ctx context.Context) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	// Fetch all records in seq order
	var records []AuditEventRecord
	if err := ds.db.WithContext(ctx).Order("seq ASC").Find(&records).Error; err != nil {
		return fmt.Errorf("failed to fetch records: %w", err)
	}

	if len(records) == 0 {
		return nil
	}

	// Validate each record's hash (including outcome and source_ip fields for tamper-detection)
	for _, record := range records {
		expectedHash := computeHash(record.Seq, record.ID, record.Tenant, record.UserUUID,
			record.Action, record.Resource, record.ResourceID, record.Outcome, record.SourceIP,
			record.Timestamp, record.Metadata, record.PrevHash)

		if expectedHash != record.Hash {
			return fmt.Errorf("hash mismatch at seq %d: expected %s, got %s",
				record.Seq, expectedHash, record.Hash)
		}

		// Validate prev_hash chain
		if record.Seq > 1 {
			var prevRecord AuditEventRecord
			if err := ds.db.Where("seq = ?", record.Seq-1).First(&prevRecord).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					return fmt.Errorf("missing previous record at seq %d", record.Seq-1)
				}
				return fmt.Errorf("failed to fetch previous record: %w", err)
			}

			if record.PrevHash != prevRecord.Hash {
				return fmt.Errorf("prev_hash mismatch at seq %d: expected %s (hash of seq %d), got %s",
					record.Seq, prevRecord.Hash, record.Seq-1, record.PrevHash)
			}
		} else if record.Seq == 1 && record.PrevHash != "" {
			return fmt.Errorf("first record (seq=1) should have empty prev_hash, got %s", record.PrevHash)
		}
	}

	return nil
}

// Close closes the database connection.
func (ds *DurableStore) Close() error {
	sqlDB, err := ds.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
