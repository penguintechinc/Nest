package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ErasureRequest is the API request/response model.
type ErasureRequest struct {
	ID           string            `json:"id"`
	Tenant       string            `json:"tenant"`
	SubjectID    string            `json:"subjectId"`
	Async        bool              `json:"async"`
	Status       string            `json:"status"`
	Backends     []string          `json:"backends,omitempty"`
	Progress     map[string]string `json:"progress,omitempty"`
	DeletedCount int               `json:"deletedCount"`
	RequestedAt  time.Time         `json:"requestedAt"`
	CompletedAt  *time.Time        `json:"completedAt,omitempty"`
	Error        string            `json:"error,omitempty"`
	Idempotency  string            `json:"idempotencyKey,omitempty"`
}

// ErasureRequestRecord is the persistent database model.
type ErasureRequestRecord struct {
	ID           string         `gorm:"index;column:id;primaryKey" json:"id"`
	Tenant       string         `gorm:"index;column:tenant" json:"tenant"`
	SubjectID    string         `gorm:"column:subject_id" json:"subjectId"`
	Async        bool           `gorm:"column:async" json:"async"`
	Status       string         `gorm:"index;column:status" json:"status"`
	Backends     datatypes.JSON `gorm:"type:text;column:backends" json:"backends,omitempty"`
	Progress     datatypes.JSON `gorm:"type:text;column:progress" json:"progress,omitempty"`
	DeletedCount int            `gorm:"column:deleted_count" json:"deletedCount"`
	RequestedAt  time.Time      `gorm:"index;column:requested_at" json:"requestedAt"`
	CompletedAt  *time.Time     `gorm:"column:completed_at" json:"completedAt,omitempty"`
	Error        string         `gorm:"type:text;column:error" json:"error,omitempty"`
	Idempotency  string         `gorm:"index;column:idempotency" json:"idempotencyKey,omitempty"`
}

// TableName sets the table name for ErasureRequestRecord.
func (ErasureRequestRecord) TableName() string {
	return "erasure_requests"
}

// ErasureStore is the durable, database-backed erasure request store.
type ErasureStore struct {
	db     *gorm.DB
	mu     sync.Mutex
	logger *zap.Logger
	eraser *ErasureOrchestrator
}

// NewErasureStore creates a new durable store backed by a database.
// It reads configuration from environment variables:
// - DB_TYPE: postgresql, mysql, or sqlite (default: sqlite)
// - DB_HOST, DB_PORT, DB_NAME, DB_USER, DB_PASS (for postgresql/mysql)
// - DB_POOL_SIZE: connection pool size (default: 10)
// - DB_MAX_RETRIES: number of retry attempts (default: 5)
// - DB_RETRY_DELAY: initial retry delay in seconds (default: 1)
func NewErasureStore(logger *zap.Logger) (*ErasureStore, error) {
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
	if err := db.AutoMigrate(&ErasureRequestRecord{}); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	store := &ErasureStore{
		db:     db,
		logger: logger,
	}

	// Initialize the erasure orchestrator with backends
	store.eraser = NewErasureOrchestrator(logger)

	return store, nil
}

// NewErasureStoreWithDB creates a store with an existing database connection (useful for testing).
func NewErasureStoreWithDB(db *gorm.DB, logger *zap.Logger) (*ErasureStore, error) {
	if err := db.AutoMigrate(&ErasureRequestRecord{}); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	store := &ErasureStore{
		db:     db,
		logger: logger,
	}
	store.eraser = NewErasureOrchestrator(logger)

	return store, nil
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
		dbName = "erasure"
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
		dbName = "erasure"
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
		dbPath = "erasure.db"
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

// CreateRequest creates a new erasure request and stores it durably.
func (s *ErasureStore) CreateRequest(r *ErasureRequest) (*ErasureRequest, error) {
	s.mu.Lock()

	// Check for idempotency
	if r.Idempotency != "" {
		var existing ErasureRequestRecord
		if err := s.db.Where("idempotency = ?", r.Idempotency).First(&existing).Error; err == nil {
			// Found existing request with same idempotency key
			s.mu.Unlock()
			return recordToRequest(&existing)
		}
	}

	// Set defaults
	if r.Backends == nil || len(r.Backends) == 0 {
		r.Backends = []string{"postgres", "kafka", "s3", "mongo", "iceberg"}
	}

	// Create request
	req := &ErasureRequest{
		ID:          fmt.Sprintf("erasure-%d", time.Now().UnixNano()),
		Tenant:      r.Tenant,
		SubjectID:   r.SubjectID,
		Async:       r.Async,
		Status:      "pending",
		Backends:    r.Backends,
		Progress:    make(map[string]string),
		RequestedAt: time.Now(),
		Idempotency: r.Idempotency,
	}

	// Convert to record and persist
	record := requestToRecord(req)
	if err := s.db.Create(&record).Error; err != nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	s.mu.Unlock()

	// Start erasure (async or sync) — outside of lock to prevent deadlock
	if r.Async {
		go s.orchestrateErasure(req)
	} else {
		s.orchestrateErasure(req)
	}

	return req, nil
}

// GetRequest retrieves an erasure request by ID.
func (s *ErasureStore) GetRequest(id string) (*ErasureRequest, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var record ErasureRequestRecord
	if err := s.db.Where("id = ?", id).First(&record).Error; err != nil {
		return nil, false
	}

	req, err := recordToRequest(&record)
	if err != nil {
		return nil, false
	}

	return req, true
}

// ListRequests lists all erasure requests for a tenant.
func (s *ErasureStore) ListRequests(tenant string) []*ErasureRequest {
	s.mu.Lock()
	defer s.mu.Unlock()

	var records []ErasureRequestRecord
	if err := s.db.Where("tenant = ?", tenant).Order("requested_at ASC").Find(&records).Error; err != nil {
		s.logger.Error("failed to list requests", zap.Error(err))
		return []*ErasureRequest{}
	}

	var result []*ErasureRequest
	for _, record := range records {
		if req, err := recordToRequest(&record); err == nil {
			result = append(result, req)
		}
	}

	return result
}

// orchestrateErasure runs the erasure for a request and persists progress.
func (s *ErasureStore) orchestrateErasure(req *ErasureRequest) {
	defaultBackends := []string{"postgres", "kafka", "s3", "mongo", "iceberg"}

	// Apply defaults if not set
	if req.Backends == nil || len(req.Backends) == 0 {
		req.Backends = defaultBackends
	}

	ctx := context.Background()

	// Invoke eraser for each backend
	totalDeleted := 0
	progress := make(map[string]string)
	backendErrors := []string{}

	for _, backend := range req.Backends {
		eraser := s.eraser.GetEraser(backend)
		if eraser == nil {
			progress[backend] = "failed"
			backendErrors = append(backendErrors, fmt.Sprintf("%s: backend not found", backend))
			continue
		}

		deleted, err := eraser.Erase(ctx, req.Tenant, req.SubjectID)
		if err != nil {
			progress[backend] = "failed"
			backendErrors = append(backendErrors, fmt.Sprintf("%s: %v", backend, err))
		} else {
			progress[backend] = "completed"
			totalDeleted += deleted
		}
	}

	// Update the request with results
	now := time.Now()
	req.DeletedCount = totalDeleted
	req.CompletedAt = &now
	req.Progress = progress

	if len(backendErrors) > 0 {
		req.Status = "failed"
		req.Error = fmt.Sprintf("erasure failed for backends: %v", backendErrors)
	} else {
		req.Status = "completed"
	}

	// Persist to database
	s.mu.Lock()
	record := requestToRecord(req)
	if err := s.db.Save(&record).Error; err != nil {
		s.logger.Error("failed to update request",
			zap.String("id", req.ID),
			zap.Error(err))
	}
	s.mu.Unlock()

	s.logger.Info("erasure request completed",
		zap.String("id", req.ID),
		zap.String("subject", req.SubjectID),
		zap.String("status", req.Status),
		zap.Int("deleted_count", req.DeletedCount),
		zap.Strings("backends", req.Backends))
}

// requestToRecord converts an ErasureRequest to a database record.
func requestToRecord(req *ErasureRequest) *ErasureRequestRecord {
	backendsJSON, _ := json.Marshal(req.Backends)
	progressJSON, _ := json.Marshal(req.Progress)

	return &ErasureRequestRecord{
		ID:           req.ID,
		Tenant:       req.Tenant,
		SubjectID:    req.SubjectID,
		Async:        req.Async,
		Status:       req.Status,
		Backends:     backendsJSON,
		Progress:     progressJSON,
		DeletedCount: req.DeletedCount,
		RequestedAt:  req.RequestedAt,
		CompletedAt:  req.CompletedAt,
		Error:        req.Error,
		Idempotency:  req.Idempotency,
	}
}

// recordToRequest converts a database record to an ErasureRequest.
func recordToRequest(record *ErasureRequestRecord) (*ErasureRequest, error) {
	var backends []string
	if len(record.Backends) > 0 {
		if err := json.Unmarshal(record.Backends, &backends); err != nil {
			return nil, fmt.Errorf("failed to unmarshal backends: %w", err)
		}
	}

	var progress map[string]string
	if len(record.Progress) > 0 {
		if err := json.Unmarshal(record.Progress, &progress); err != nil {
			return nil, fmt.Errorf("failed to unmarshal progress: %w", err)
		}
	}

	if progress == nil {
		progress = make(map[string]string)
	}

	return &ErasureRequest{
		ID:           record.ID,
		Tenant:       record.Tenant,
		SubjectID:    record.SubjectID,
		Async:        record.Async,
		Status:       record.Status,
		Backends:     backends,
		Progress:     progress,
		DeletedCount: record.DeletedCount,
		RequestedAt:  record.RequestedAt,
		CompletedAt:  record.CompletedAt,
		Error:        record.Error,
		Idempotency:  record.Idempotency,
	}, nil
}

// Close closes the database connection.
func (s *ErasureStore) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
