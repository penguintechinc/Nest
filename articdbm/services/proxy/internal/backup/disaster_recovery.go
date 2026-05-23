package backup

import (
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

type DisasterRecoveryManager struct {
	logger       *zap.Logger
	redisClient  *redis.Client
	backupPath   string
	encryptionKey []byte

	// Configuration
	retentionDays     int
	backupInterval    time.Duration
	compressionLevel  int
	replicationSites  []ReplicationSite

	// Runtime state
	isRunning    bool
	mu           sync.RWMutex
	lastBackup   time.Time
	backupStats  BackupStatistics
}

type ReplicationSite struct {
	Name         string `json:"name"`
	Type         string `json:"type"` // "s3", "gcs", "azure", "ftp", "local"
	Endpoint     string `json:"endpoint"`
	Credentials  string `json:"credentials"`
	Bucket       string `json:"bucket,omitempty"`
	Region       string `json:"region,omitempty"`
	Enabled      bool   `json:"enabled"`
	Priority     int    `json:"priority"`
	MaxRetries   int    `json:"max_retries"`
	RetryDelay   time.Duration `json:"retry_delay"`
}

type BackupMetadata struct {
	Timestamp      time.Time     `json:"timestamp"`
	Size           int64         `json:"size"`
	CompressedSize int64         `json:"compressed_size"`
	Checksum       string        `json:"checksum"`
	Type           string        `json:"type"` // "full", "incremental", "differential"
	Components     []string      `json:"components"`
	Version        string        `json:"version"`
	Encrypted      bool          `json:"encrypted"`
	CompressionRatio float64     `json:"compression_ratio"`
	Duration       time.Duration `json:"duration"`
}

type BackupStatistics struct {
	TotalBackups     int           `json:"total_backups"`
	SuccessfulBackups int          `json:"successful_backups"`
	FailedBackups    int           `json:"failed_backups"`
	TotalSize        int64         `json:"total_size"`
	AverageSize      int64         `json:"average_size"`
	LastBackupTime   time.Time     `json:"last_backup_time"`
	LastBackupStatus string        `json:"last_backup_status"`
	RetentionCleanups int          `json:"retention_cleanups"`
	ReplicationStatus map[string]string `json:"replication_status"`
}

type RestorePoint struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Timestamp    time.Time       `json:"timestamp"`
	Type         string          `json:"type"`
	Components   []string        `json:"components"`
	Size         int64           `json:"size"`
	Checksum     string          `json:"checksum"`
	Metadata     BackupMetadata  `json:"metadata"`
	Location     string          `json:"location"`
	Verified     bool            `json:"verified"`
	VerifiedAt   time.Time       `json:"verified_at"`
}

type RecoveryPlan struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Description      string         `json:"description"`
	RestorePoints    []RestorePoint `json:"restore_points"`
	RecoverySteps    []RecoveryStep `json:"recovery_steps"`
	EstimatedRTO     time.Duration  `json:"estimated_rto"` // Recovery Time Objective
	EstimatedRPO     time.Duration  `json:"estimated_rpo"` // Recovery Point Objective
	AutomationLevel  string         `json:"automation_level"` // "manual", "semi-automatic", "automatic"
	ValidationSteps  []string       `json:"validation_steps"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

type RecoveryStep struct {
	Order        int           `json:"order"`
	Name         string        `json:"name"`
	Type         string        `json:"type"` // "restore_data", "restart_service", "verify_health", "custom"
	Command      string        `json:"command,omitempty"`
	Timeout      time.Duration `json:"timeout"`
	RetryCount   int           `json:"retry_count"`
	Dependencies []string      `json:"dependencies"`
	Automated    bool          `json:"automated"`
}

func NewDisasterRecoveryManager(logger *zap.Logger, redisClient *redis.Client, config map[string]interface{}) (*DisasterRecoveryManager, error) {
	backupPath := "/var/lib/articdbm/backups"
	if path, ok := config["backup_path"].(string); ok {
		backupPath = path
	}

	encryptionKey := make([]byte, 32)
	if keyStr, ok := config["encryption_key"].(string); ok && len(keyStr) >= 32 {
		copy(encryptionKey, []byte(keyStr)[:32])
	} else {
		if _, err := rand.Read(encryptionKey); err != nil {
			return nil, fmt.Errorf("failed to generate encryption key: %w", err)
		}
	}

	retentionDays := 30
	if days, ok := config["retention_days"].(int); ok {
		retentionDays = days
	}

	backupInterval := 6 * time.Hour
	if interval, ok := config["backup_interval"].(string); ok {
		if parsed, err := time.ParseDuration(interval); err == nil {
			backupInterval = parsed
		}
	}

	compressionLevel := 6
	if level, ok := config["compression_level"].(int); ok && level >= 1 && level <= 9 {
		compressionLevel = level
	}

	var replicationSites []ReplicationSite
	if sites, ok := config["replication_sites"].([]interface{}); ok {
		for _, site := range sites {
			if siteMap, ok := site.(map[string]interface{}); ok {
				rs := ReplicationSite{
					Name:        siteMap["name"].(string),
					Type:        siteMap["type"].(string),
					Endpoint:    siteMap["endpoint"].(string),
					Credentials: siteMap["credentials"].(string),
					Enabled:     true,
					Priority:    1,
					MaxRetries:  3,
					RetryDelay:  30 * time.Second,
				}
				if bucket, ok := siteMap["bucket"].(string); ok {
					rs.Bucket = bucket
				}
				if region, ok := siteMap["region"].(string); ok {
					rs.Region = region
				}
				if enabled, ok := siteMap["enabled"].(bool); ok {
					rs.Enabled = enabled
				}
				if priority, ok := siteMap["priority"].(int); ok {
					rs.Priority = priority
				}
				replicationSites = append(replicationSites, rs)
			}
		}
	}

	// Create backup directory
	if err := os.MkdirAll(backupPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create backup directory: %w", err)
	}

	drm := &DisasterRecoveryManager{
		logger:           logger,
		redisClient:      redisClient,
		backupPath:       backupPath,
		encryptionKey:    encryptionKey,
		retentionDays:    retentionDays,
		backupInterval:   backupInterval,
		compressionLevel: compressionLevel,
		replicationSites: replicationSites,
		backupStats: BackupStatistics{
			ReplicationStatus: make(map[string]string),
		},
	}

	// Initialize replication status
	for _, site := range replicationSites {
		drm.backupStats.ReplicationStatus[site.Name] = "unknown"
	}

	return drm, nil
}

func (drm *DisasterRecoveryManager) Start(ctx context.Context) error {
	drm.mu.Lock()
	defer drm.mu.Unlock()

	if drm.isRunning {
		return fmt.Errorf("disaster recovery manager already running")
	}

	drm.isRunning = true
	drm.logger.Info("Starting disaster recovery manager",
		zap.String("backup_path", drm.backupPath),
		zap.Duration("backup_interval", drm.backupInterval),
		zap.Int("retention_days", drm.retentionDays))

	// Start backup scheduler
	go drm.backupScheduler(ctx)

	// Start replication monitor
	go drm.replicationMonitor(ctx)

	// Start cleanup scheduler
	go drm.cleanupScheduler(ctx)

	return nil
}

func (drm *DisasterRecoveryManager) Stop() error {
	drm.mu.Lock()
	defer drm.mu.Unlock()

	if !drm.isRunning {
		return fmt.Errorf("disaster recovery manager not running")
	}

	drm.isRunning = false
	drm.logger.Info("Stopping disaster recovery manager")

	return nil
}

func (drm *DisasterRecoveryManager) backupScheduler(ctx context.Context) {
	ticker := time.NewTicker(drm.backupInterval)
	defer ticker.Stop()

	// Perform initial backup
	if err := drm.performBackup(ctx, "scheduled"); err != nil {
		drm.logger.Error("Initial backup failed", zap.Error(err))
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			drm.mu.RLock()
			running := drm.isRunning
			drm.mu.RUnlock()

			if !running {
				return
			}

			if err := drm.performBackup(ctx, "scheduled"); err != nil {
				drm.logger.Error("Scheduled backup failed", zap.Error(err))
			}
		}
	}
}

func (drm *DisasterRecoveryManager) performBackup(ctx context.Context, backupType string) error {
	start := time.Now()

	// Generate backup ID
	backupID := fmt.Sprintf("backup_%s_%d", backupType, start.Unix())
	backupDir := filepath.Join(drm.backupPath, backupID)

	drm.logger.Info("Starting backup",
		zap.String("backup_id", backupID),
		zap.String("type", backupType))

	// Create backup directory
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	var components []string
	var totalSize int64

	// Backup Redis data
	if err := drm.backupRedis(ctx, backupDir); err != nil {
		drm.logger.Error("Redis backup failed", zap.Error(err))
	} else {
		components = append(components, "redis")
		if info, err := os.Stat(filepath.Join(backupDir, "redis.backup")); err == nil {
			totalSize += info.Size()
		}
	}

	// Backup configuration files
	if err := drm.backupConfiguration(ctx, backupDir); err != nil {
		drm.logger.Error("Configuration backup failed", zap.Error(err))
	} else {
		components = append(components, "configuration")
		if info, err := os.Stat(filepath.Join(backupDir, "config.backup")); err == nil {
			totalSize += info.Size()
		}
	}

	// Backup audit logs
	if err := drm.backupAuditLogs(ctx, backupDir); err != nil {
		drm.logger.Error("Audit logs backup failed", zap.Error(err))
	} else {
		components = append(components, "audit_logs")
		if info, err := os.Stat(filepath.Join(backupDir, "audit.backup")); err == nil {
			totalSize += info.Size()
		}
	}

	// Create compressed archive
	archivePath := backupDir + ".tar.gz"
	compressedSize, err := drm.createCompressedArchive(backupDir, archivePath)
	if err != nil {
		return fmt.Errorf("failed to create archive: %w", err)
	}

	// Calculate checksum
	checksum, err := drm.calculateChecksum(archivePath)
	if err != nil {
		return fmt.Errorf("failed to calculate checksum: %w", err)
	}

	// Encrypt archive if encryption is enabled
	encryptedPath := archivePath
	if len(drm.encryptionKey) > 0 {
		encryptedPath = archivePath + ".enc"
		if err := drm.encryptFile(archivePath, encryptedPath); err != nil {
			drm.logger.Error("Failed to encrypt backup", zap.Error(err))
		} else {
			os.Remove(archivePath)
		}
	}

	// Create metadata
	metadata := BackupMetadata{
		Timestamp:        start,
		Size:             totalSize,
		CompressedSize:   compressedSize,
		Checksum:         checksum,
		Type:             "full",
		Components:       components,
		Version:          "1.2.0",
		Encrypted:        len(drm.encryptionKey) > 0,
		CompressionRatio: float64(totalSize-compressedSize) / float64(totalSize) * 100,
		Duration:         time.Since(start),
	}

	// Save metadata
	metadataPath := encryptedPath + ".meta"
	if err := drm.saveMetadata(metadata, metadataPath); err != nil {
		drm.logger.Error("Failed to save metadata", zap.Error(err))
	}

	// Clean up temporary directory
	os.RemoveAll(backupDir)

	// Update statistics
	drm.updateBackupStats(true, totalSize)

	// Replicate to remote sites
	go drm.replicateBackup(ctx, encryptedPath, metadata)

	drm.logger.Info("Backup completed successfully",
		zap.String("backup_id", backupID),
		zap.Duration("duration", time.Since(start)),
		zap.Int64("size", totalSize),
		zap.Int64("compressed_size", compressedSize),
		zap.Float64("compression_ratio", metadata.CompressionRatio),
		zap.Strings("components", components))

	return nil
}

func (drm *DisasterRecoveryManager) backupRedis(ctx context.Context, backupDir string) error {
	// Create Redis snapshot
	result := drm.redisClient.BgSave(ctx)
	if err := result.Err(); err != nil {
		return fmt.Errorf("failed to create Redis snapshot: %w", err)
	}

	// Wait for snapshot to complete
	for {
		lastsave := drm.redisClient.LastSave(ctx)
		if lastsave.Err() != nil {
			return fmt.Errorf("failed to check snapshot status: %w", lastsave.Err())
		}

		// Check if snapshot is newer than backup start
		if lastsave.Val().After(time.Now().Add(-5 * time.Minute)) {
			break
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}

	// Export Redis data as JSON for portability
	backupFile := filepath.Join(backupDir, "redis.backup")
	file, err := os.Create(backupFile)
	if err != nil {
		return fmt.Errorf("failed to create backup file: %w", err)
	}
	defer file.Close()

	writer := gzip.NewWriter(file)
	defer writer.Close()

	// Scan all keys and export
	iter := drm.redisClient.Scan(ctx, 0, "*", 1000).Iterator()
	encoder := json.NewEncoder(writer)

	backupData := make(map[string]interface{})

	for iter.Next(ctx) {
		key := iter.Val()

		// Get key type
		keyType := drm.redisClient.Type(ctx, key)
		if keyType.Err() != nil {
			continue
		}

		switch keyType.Val() {
		case "string":
			val := drm.redisClient.Get(ctx, key)
			if val.Err() == nil {
				backupData[key] = map[string]interface{}{
					"type":  "string",
					"value": val.Val(),
				}
			}
		case "hash":
			val := drm.redisClient.HGetAll(ctx, key)
			if val.Err() == nil {
				backupData[key] = map[string]interface{}{
					"type":  "hash",
					"value": val.Val(),
				}
			}
		case "list":
			val := drm.redisClient.LRange(ctx, key, 0, -1)
			if val.Err() == nil {
				backupData[key] = map[string]interface{}{
					"type":  "list",
					"value": val.Val(),
				}
			}
		case "set":
			val := drm.redisClient.SMembers(ctx, key)
			if val.Err() == nil {
				backupData[key] = map[string]interface{}{
					"type":  "set",
					"value": val.Val(),
				}
			}
		case "zset":
			val := drm.redisClient.ZRangeWithScores(ctx, key, 0, -1)
			if val.Err() == nil {
				backupData[key] = map[string]interface{}{
					"type":  "zset",
					"value": val.Val(),
				}
			}
		}

		// Get TTL
		ttl := drm.redisClient.TTL(ctx, key)
		if ttl.Err() == nil && ttl.Val() > 0 {
			if entry, ok := backupData[key].(map[string]interface{}); ok {
				entry["ttl"] = ttl.Val().Seconds()
			}
		}
	}

	return encoder.Encode(backupData)
}

func (drm *DisasterRecoveryManager) backupConfiguration(ctx context.Context, backupDir string) error {
	configBackup := filepath.Join(backupDir, "config.backup")
	file, err := os.Create(configBackup)
	if err != nil {
		return fmt.Errorf("failed to create config backup: %w", err)
	}
	defer file.Close()

	writer := gzip.NewWriter(file)
	defer writer.Close()

	// Backup environment variables
	config := make(map[string]interface{})
	config["environment"] = os.Environ()
	config["timestamp"] = time.Now().Unix()

	// Add any additional configuration data here
	// This could include database schemas, user configurations, etc.

	encoder := json.NewEncoder(writer)
	return encoder.Encode(config)
}

func (drm *DisasterRecoveryManager) backupAuditLogs(ctx context.Context, backupDir string) error {
	auditBackup := filepath.Join(backupDir, "audit.backup")
	file, err := os.Create(auditBackup)
	if err != nil {
		return fmt.Errorf("failed to create audit backup: %w", err)
	}
	defer file.Close()

	writer := gzip.NewWriter(file)
	defer writer.Close()

	// For now, create a placeholder structure
	// In a real implementation, this would backup audit logs from PostgreSQL
	auditData := map[string]interface{}{
		"timestamp": time.Now().Unix(),
		"note":      "Audit log backup placeholder - implement based on your audit log storage",
	}

	encoder := json.NewEncoder(writer)
	return encoder.Encode(auditData)
}

func (drm *DisasterRecoveryManager) createCompressedArchive(sourceDir, archivePath string) (int64, error) {
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		return 0, err
	}
	defer archiveFile.Close()

	gzipWriter, err := gzip.NewWriterLevel(archiveFile, drm.compressionLevel)
	if err != nil {
		return 0, err
	}
	defer gzipWriter.Close()

	// Simple implementation - in production, use tar with gzip
	// For now, just compress the first file found
	files, err := os.ReadDir(sourceDir)
	if err != nil {
		return 0, err
	}

	var totalSize int64
	for _, file := range files {
		if !file.IsDir() {
			filePath := filepath.Join(sourceDir, file.Name())
			sourceFile, err := os.Open(filePath)
			if err != nil {
				continue
			}

			written, err := io.Copy(gzipWriter, sourceFile)
			sourceFile.Close()
			if err != nil {
				return 0, err
			}
			totalSize += written
		}
	}

	info, err := archiveFile.Stat()
	if err != nil {
		return totalSize, err
	}

	return info.Size(), nil
}

func (drm *DisasterRecoveryManager) calculateChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

func (drm *DisasterRecoveryManager) encryptFile(inputPath, outputPath string) error {
	inputFile, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer inputFile.Close()

	outputFile, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer outputFile.Close()

	block, err := aes.NewCipher(drm.encryptionKey)
	if err != nil {
		return err
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	// Create nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}

	// Write nonce to file
	if _, err := outputFile.Write(nonce); err != nil {
		return err
	}

	// Read input file
	plaintext, err := io.ReadAll(inputFile)
	if err != nil {
		return err
	}

	// Encrypt and write
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	_, err = outputFile.Write(ciphertext)

	return err
}

func (drm *DisasterRecoveryManager) saveMetadata(metadata BackupMetadata, metadataPath string) error {
	file, err := os.Create(metadataPath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(metadata)
}

func (drm *DisasterRecoveryManager) updateBackupStats(success bool, size int64) {
	drm.mu.Lock()
	defer drm.mu.Unlock()

	drm.backupStats.TotalBackups++
	drm.backupStats.LastBackupTime = time.Now()

	if success {
		drm.backupStats.SuccessfulBackups++
		drm.backupStats.LastBackupStatus = "success"
		drm.backupStats.TotalSize += size
		if drm.backupStats.SuccessfulBackups > 0 {
			drm.backupStats.AverageSize = drm.backupStats.TotalSize / int64(drm.backupStats.SuccessfulBackups)
		}
	} else {
		drm.backupStats.FailedBackups++
		drm.backupStats.LastBackupStatus = "failed"
	}
}

func (drm *DisasterRecoveryManager) replicateBackup(ctx context.Context, backupPath string, metadata BackupMetadata) {
	for _, site := range drm.replicationSites {
		if !site.Enabled {
			continue
		}

		drm.logger.Info("Replicating backup to site",
			zap.String("site", site.Name),
			zap.String("type", site.Type))

		err := drm.replicateToSite(ctx, site, backupPath, metadata)
		if err != nil {
			drm.logger.Error("Backup replication failed",
				zap.String("site", site.Name),
				zap.Error(err))
			drm.mu.Lock()
			drm.backupStats.ReplicationStatus[site.Name] = "failed"
			drm.mu.Unlock()
		} else {
			drm.logger.Info("Backup replication successful",
				zap.String("site", site.Name))
			drm.mu.Lock()
			drm.backupStats.ReplicationStatus[site.Name] = "success"
			drm.mu.Unlock()
		}
	}
}

func (drm *DisasterRecoveryManager) replicateToSite(ctx context.Context, site ReplicationSite, backupPath string, metadata BackupMetadata) error {
	// Placeholder implementation for different site types
	switch site.Type {
	case "local":
		return drm.replicateToLocal(site, backupPath)
	case "s3":
		return drm.replicateToS3(site, backupPath, metadata)
	case "gcs":
		return drm.replicateToGCS(site, backupPath, metadata)
	case "azure":
		return drm.replicateToAzure(site, backupPath, metadata)
	default:
		return fmt.Errorf("unsupported replication site type: %s", site.Type)
	}
}

func (drm *DisasterRecoveryManager) replicateToLocal(site ReplicationSite, backupPath string) error {
	destPath := filepath.Join(site.Endpoint, filepath.Base(backupPath))

	// Create destination directory
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	// Copy file
	src, err := os.Open(backupPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

func (drm *DisasterRecoveryManager) replicateToS3(site ReplicationSite, backupPath string, metadata BackupMetadata) error {
	// Placeholder for AWS S3 replication
	// In a real implementation, use AWS SDK to upload to S3
	drm.logger.Info("S3 replication not implemented - placeholder")
	return nil
}

func (drm *DisasterRecoveryManager) replicateToGCS(site ReplicationSite, backupPath string, metadata BackupMetadata) error {
	// Placeholder for Google Cloud Storage replication
	// In a real implementation, use GCS client library
	drm.logger.Info("GCS replication not implemented - placeholder")
	return nil
}

func (drm *DisasterRecoveryManager) replicateToAzure(site ReplicationSite, backupPath string, metadata BackupMetadata) error {
	// Placeholder for Azure Blob Storage replication
	// In a real implementation, use Azure SDK
	drm.logger.Info("Azure replication not implemented - placeholder")
	return nil
}

func (drm *DisasterRecoveryManager) replicationMonitor(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			drm.mu.RLock()
			running := drm.isRunning
			drm.mu.RUnlock()

			if !running {
				return
			}

			// Check replication site health
			for _, site := range drm.replicationSites {
				if !site.Enabled {
					continue
				}

				// Perform health check
				if err := drm.checkSiteHealth(ctx, site); err != nil {
					drm.logger.Warn("Replication site health check failed",
						zap.String("site", site.Name),
						zap.Error(err))
					drm.mu.Lock()
					drm.backupStats.ReplicationStatus[site.Name] = "unhealthy"
					drm.mu.Unlock()
				} else {
					drm.mu.Lock()
					if drm.backupStats.ReplicationStatus[site.Name] != "failed" {
						drm.backupStats.ReplicationStatus[site.Name] = "healthy"
					}
					drm.mu.Unlock()
				}
			}
		}
	}
}

func (drm *DisasterRecoveryManager) checkSiteHealth(ctx context.Context, site ReplicationSite) error {
	// Placeholder health check implementation
	// In real implementation, check connectivity and permissions
	switch site.Type {
	case "local":
		// Check if directory is accessible
		if _, err := os.Stat(site.Endpoint); err != nil {
			return err
		}
	case "s3", "gcs", "azure":
		// Check cloud connectivity (placeholder)
		drm.logger.Debug("Cloud site health check not implemented",
			zap.String("site", site.Name),
			zap.String("type", site.Type))
	}

	return nil
}

func (drm *DisasterRecoveryManager) cleanupScheduler(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			drm.mu.RLock()
			running := drm.isRunning
			drm.mu.RUnlock()

			if !running {
				return
			}

			drm.cleanupOldBackups()
		}
	}
}

func (drm *DisasterRecoveryManager) cleanupOldBackups() {
	cutoff := time.Now().AddDate(0, 0, -drm.retentionDays)

	files, err := os.ReadDir(drm.backupPath)
	if err != nil {
		drm.logger.Error("Failed to read backup directory", zap.Error(err))
		return
	}

	cleaned := 0
	for _, file := range files {
		if !file.IsDir() && strings.HasPrefix(file.Name(), "backup_") {
			info, err := file.Info()
			if err != nil {
				continue
			}

			if info.ModTime().Before(cutoff) {
				filePath := filepath.Join(drm.backupPath, file.Name())
				if err := os.Remove(filePath); err != nil {
					drm.logger.Error("Failed to remove old backup",
						zap.String("file", filePath),
						zap.Error(err))
				} else {
					cleaned++
					drm.logger.Debug("Removed old backup",
						zap.String("file", file.Name()),
						zap.Time("mod_time", info.ModTime()))
				}
			}
		}
	}

	if cleaned > 0 {
		drm.mu.Lock()
		drm.backupStats.RetentionCleanups += cleaned
		drm.mu.Unlock()

		drm.logger.Info("Cleaned up old backups",
			zap.Int("count", cleaned),
			zap.Int("retention_days", drm.retentionDays))
	}
}

func (drm *DisasterRecoveryManager) GetBackupStatistics() BackupStatistics {
	drm.mu.RLock()
	defer drm.mu.RUnlock()

	stats := drm.backupStats
	stats.ReplicationStatus = make(map[string]string)
	for k, v := range drm.backupStats.ReplicationStatus {
		stats.ReplicationStatus[k] = v
	}

	return stats
}

func (drm *DisasterRecoveryManager) TriggerBackup(ctx context.Context, backupType string) error {
	return drm.performBackup(ctx, backupType)
}

func (drm *DisasterRecoveryManager) ListRestorePoints() ([]RestorePoint, error) {
	files, err := os.ReadDir(drm.backupPath)
	if err != nil {
		return nil, err
	}

	var restorePoints []RestorePoint

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".meta") {
			metadataPath := filepath.Join(drm.backupPath, file.Name())
			backupPath := strings.TrimSuffix(metadataPath, ".meta")

			// Load metadata
			metaFile, err := os.Open(metadataPath)
			if err != nil {
				continue
			}

			var metadata BackupMetadata
			if err := json.NewDecoder(metaFile).Decode(&metadata); err != nil {
				metaFile.Close()
				continue
			}
			metaFile.Close()

			// Check if backup file exists
			if _, err := os.Stat(backupPath); err != nil {
				continue
			}

			restorePoint := RestorePoint{
				ID:         strings.TrimSuffix(file.Name(), ".meta"),
				Name:       fmt.Sprintf("Backup %s", metadata.Timestamp.Format("2006-01-02 15:04:05")),
				Timestamp:  metadata.Timestamp,
				Type:       metadata.Type,
				Components: metadata.Components,
				Size:       metadata.Size,
				Checksum:   metadata.Checksum,
				Metadata:   metadata,
				Location:   backupPath,
				Verified:   false,
			}

			restorePoints = append(restorePoints, restorePoint)
		}
	}

	return restorePoints, nil
}