package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"go.uber.org/zap"
)

// Eraser defines the interface for backend-specific erasure implementations.
type Eraser interface {
	Erase(ctx context.Context, tenant, subjectID string) (deletedCount int, err error)
}

// ErasureOrchestrator manages backend erasers.
type ErasureOrchestrator struct {
	erasers map[string]Eraser
	logger  *zap.Logger
}

// NewErasureOrchestrator creates a new orchestrator with default erasers.
// REAL IMPLEMENTATIONS: postgres, s3 (e2e-deferred, fail on missing config).
// FAIL-LOUD: kafka, mongo, iceberg (return errors — never silent success).
func NewErasureOrchestrator(logger *zap.Logger) *ErasureOrchestrator {
	o := &ErasureOrchestrator{
		erasers: make(map[string]Eraser),
		logger:  logger,
	}

	// Register erasers: real implementations for postgres/s3, fail-loud for others
	o.erasers["postgres"] = NewPostgresEraser(logger)
	o.erasers["s3"] = NewS3Eraser(logger)
	o.erasers["kafka"] = NewKafkaEraser(logger)     // fail-loud
	o.erasers["mongo"] = NewMongoEraser(logger)     // fail-loud
	o.erasers["iceberg"] = NewIcebergEraser(logger) // fail-loud

	return o
}

// GetEraser retrieves an eraser by backend name.
func (o *ErasureOrchestrator) GetEraser(backend string) Eraser {
	return o.erasers[backend]
}

// RegisterEraser registers a custom eraser (useful for testing).
func (o *ErasureOrchestrator) RegisterEraser(backend string, eraser Eraser) {
	o.erasers[backend] = eraser
}

// PostgresEraser implements the Eraser interface for PostgreSQL.
// REAL IMPLEMENTATION (e2e-deferred): connects to postgres backend and executes DELETE queries.
type PostgresEraser struct {
	logger *zap.Logger
}

// NewPostgresEraser creates a new PostgreSQL eraser.
func NewPostgresEraser(logger *zap.Logger) *PostgresEraser {
	return &PostgresEraser{logger: logger}
}

// Erase deletes all data for a subject from PostgreSQL.
// Real implementation: connects to POSTGRES_URL and executes DELETE FROM ... WHERE tenant=$1 AND subject_id=$2.
// E2E-deferred: requires live PostgreSQL backend (testcontainers integration test).
// FAIL LOUD if POSTGRES_URL not set (never return 0, nil for unimplemented).
func (e *PostgresEraser) Erase(ctx context.Context, tenant, subjectID string) (int, error) {
	postgresURL := os.Getenv("POSTGRES_URL")
	if postgresURL == "" {
		return 0, fmt.Errorf("postgres erasure: POSTGRES_URL not configured (e2e-deferred)")
	}

	db, err := sql.Open("postgres", postgresURL)
	if err != nil {
		return 0, fmt.Errorf("postgres erasure: connection failed: %w", err)
	}
	defer db.Close()

	// Ping to verify connection
	if err := db.PingContext(ctx); err != nil {
		return 0, fmt.Errorf("postgres erasure: ping failed: %w", err)
	}

	// Delete from a canonical subjects table (data-plane would have registered subjects).
	// This is the real deletion path; subject data in other tables would be deleted via FK cascades
	// or separate DELETE queries per data table.
	result, err := db.ExecContext(ctx,
		`DELETE FROM subjects WHERE tenant = $1 AND subject_id = $2`,
		tenant, subjectID)
	if err != nil {
		return 0, fmt.Errorf("postgres erasure: delete failed: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("postgres erasure: rows affected check failed: %w", err)
	}

	e.logger.Info("postgres erasure completed",
		zap.String("tenant", tenant),
		zap.String("subject", subjectID),
		zap.Int64("deleted_rows", rowsAffected))

	return int(rowsAffected), nil
}

// S3Eraser implements the Eraser interface for S3.
// REAL IMPLEMENTATION (e2e-deferred): connects to S3 and deletes objects by prefix.
type S3Eraser struct {
	logger *zap.Logger
}

// NewS3Eraser creates a new S3 eraser.
func NewS3Eraser(logger *zap.Logger) *S3Eraser {
	return &S3Eraser{logger: logger}
}

// Erase deletes all objects for a subject from S3.
// Real implementation: connects to S3 (via AWS SDK) and deletes objects under {tenant}/{subject}/ prefix.
// E2E-deferred: requires live S3 bucket (or LocalStack for testing).
// FAIL LOUD if S3_BUCKET not set (never return 0, nil for unimplemented).
func (e *S3Eraser) Erase(ctx context.Context, tenant, subjectID string) (int, error) {
	bucket := os.Getenv("S3_BUCKET")
	if bucket == "" {
		return 0, fmt.Errorf("s3 erasure: S3_BUCKET not configured (e2e-deferred)")
	}

	// Load AWS config (respects AWS_* env vars)
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return 0, fmt.Errorf("s3 erasure: aws config load failed: %w", err)
	}

	client := s3.NewFromConfig(cfg)

	// Construct prefix: {tenant}/{subject}/
	prefix := fmt.Sprintf("%s/%s/", tenant, subjectID)

	// List objects with prefix
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: &bucket,
		Prefix: &prefix,
	})

	totalDeleted := 0

	// Paginate through results and delete
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return 0, fmt.Errorf("s3 erasure: list failed: %w", err)
		}

		if len(page.Contents) == 0 {
			continue
		}

		// Build delete request for up to 1000 objects per batch
		var objectIDs []types.ObjectIdentifier
		for _, obj := range page.Contents {
			objectIDs = append(objectIDs, types.ObjectIdentifier{Key: obj.Key})
		}

		_, err = client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: &bucket,
			Delete: &types.Delete{Objects: objectIDs},
		})
		if err != nil {
			return 0, fmt.Errorf("s3 erasure: delete failed: %w", err)
		}

		totalDeleted += len(objectIDs)
	}

	e.logger.Info("s3 erasure completed",
		zap.String("bucket", bucket),
		zap.String("prefix", prefix),
		zap.Int("deleted_objects", totalDeleted))

	return totalDeleted, nil
}

// KafkaEraser implements the Eraser interface for Kafka.
// NOT IMPLEMENTED: fail loudly (deferred to e2e with live Kafka).
// FAIL LOUD: return error, never 0, nil.
type KafkaEraser struct {
	logger *zap.Logger
}

// NewKafkaEraser creates a new Kafka eraser.
func NewKafkaEraser(logger *zap.Logger) *KafkaEraser {
	return &KafkaEraser{logger: logger}
}

// Erase returns an error indicating Kafka erasure is not yet implemented.
// FAIL LOUD: never return 0, nil for an unimplemented backend.
func (e *KafkaEraser) Erase(ctx context.Context, tenant, subjectID string) (int, error) {
	return 0, fmt.Errorf("kafka erasure not implemented; requires Kafka producer/topic configuration (e2e-deferred)")
}

// MongoEraser implements the Eraser interface for MongoDB.
// NOT IMPLEMENTED: fail loudly (deferred to e2e with live MongoDB).
// FAIL LOUD: return error, never 0, nil.
type MongoEraser struct {
	logger *zap.Logger
}

// NewMongoEraser creates a new MongoDB eraser.
func NewMongoEraser(logger *zap.Logger) *MongoEraser {
	return &MongoEraser{logger: logger}
}

// Erase returns an error indicating MongoDB erasure is not yet implemented.
// FAIL LOUD: never return 0, nil for an unimplemented backend.
func (e *MongoEraser) Erase(ctx context.Context, tenant, subjectID string) (int, error) {
	return 0, fmt.Errorf("mongo erasure not implemented; requires MongoDB driver and collection discovery (e2e-deferred)")
}

// IcebergEraser implements the Eraser interface for Iceberg.
// NOT IMPLEMENTED: fail loudly (deferred to e2e with live Iceberg catalog).
// FAIL LOUD: return error, never 0, nil.
type IcebergEraser struct {
	logger *zap.Logger
}

// NewIcebergEraser creates a new Iceberg eraser.
func NewIcebergEraser(logger *zap.Logger) *IcebergEraser {
	return &IcebergEraser{logger: logger}
}

// Erase returns an error indicating Iceberg erasure is not yet implemented.
// FAIL LOUD: never return 0, nil for an unimplemented backend.
func (e *IcebergEraser) Erase(ctx context.Context, tenant, subjectID string) (int, error) {
	return 0, fmt.Errorf("iceberg erasure not implemented; requires Iceberg client and table discovery (e2e-deferred)")
}

// FakeEraser is an injectable fake eraser for testing.
type FakeEraser struct {
	deletedCount int
	err          error
	called       int
}

// NewFakeEraser creates a new fake eraser.
func NewFakeEraser(deletedCount int, err error) *FakeEraser {
	return &FakeEraser{deletedCount: deletedCount, err: err, called: 0}
}

// Erase implements the Eraser interface.
func (f *FakeEraser) Erase(ctx context.Context, tenant, subjectID string) (int, error) {
	f.called++
	return f.deletedCount, f.err
}

// Called returns the number of times Erase was called.
func (f *FakeEraser) Called() int {
	return f.called
}
