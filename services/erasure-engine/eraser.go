package main

import (
	"context"

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
func NewErasureOrchestrator(logger *zap.Logger) *ErasureOrchestrator {
	o := &ErasureOrchestrator{
		erasers: make(map[string]Eraser),
		logger:  logger,
	}

	// Register default erasers
	o.erasers["postgres"] = NewPostgresEraser(logger)
	o.erasers["kafka"] = NewKafkaEraser(logger)
	o.erasers["s3"] = NewS3Eraser(logger)
	o.erasers["mongo"] = NewMongoEraser(logger)
	o.erasers["iceberg"] = NewIcebergEraser(logger)

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
type PostgresEraser struct {
	logger *zap.Logger
}

// NewPostgresEraser creates a new PostgreSQL eraser.
func NewPostgresEraser(logger *zap.Logger) *PostgresEraser {
	return &PostgresEraser{logger: logger}
}

// Erase deletes all data for a subject from PostgreSQL.
// For now, a real implementation would connect to the postgres backend and DELETE by subject/tenant.
func (e *PostgresEraser) Erase(ctx context.Context, tenant, subjectID string) (int, error) {
	// TODO: Connect to postgres backend and execute real DELETE queries.
	// This would require:
	// - Reading connection details from env vars (POSTGRES_HOST, POSTGRES_DB, etc.)
	// - Building a database connection
	// - Identifying tables with subject/tenant columns
	// - Executing DELETE FROM ... WHERE subject_id = ? AND tenant = ?
	// - Counting deleted rows and returning the total
	e.logger.Debug("postgres erasure (stub)", zap.String("tenant", tenant), zap.String("subject", subjectID))
	return 0, nil
}

// KafkaEraser implements the Eraser interface for Kafka.
type KafkaEraser struct {
	logger *zap.Logger
}

// NewKafkaEraser creates a new Kafka eraser.
func NewKafkaEraser(logger *zap.Logger) *KafkaEraser {
	return &KafkaEraser{logger: logger}
}

// Erase publishes tombstone/delete messages for a subject to Kafka.
// For now, a real implementation would connect to kafka and publish deletion events.
func (e *KafkaEraser) Erase(ctx context.Context, tenant, subjectID string) (int, error) {
	// TODO: Connect to kafka and publish deletion events/tombstones.
	// This would require:
	// - Reading kafka broker details from env vars
	// - Identifying topics with subject data
	// - Publishing deletion/tombstone messages
	// - Counting published messages
	e.logger.Debug("kafka erasure (stub)", zap.String("tenant", tenant), zap.String("subject", subjectID))
	return 0, nil
}

// S3Eraser implements the Eraser interface for S3.
type S3Eraser struct {
	logger *zap.Logger
}

// NewS3Eraser creates a new S3 eraser.
func NewS3Eraser(logger *zap.Logger) *S3Eraser {
	return &S3Eraser{logger: logger}
}

// Erase deletes all objects for a subject from S3.
// For now, a real implementation would connect to S3 and delete by prefix.
func (e *S3Eraser) Erase(ctx context.Context, tenant, subjectID string) (int, error) {
	// TODO: Connect to S3 and delete objects by prefix (e.g., s3://{bucket}/{tenant}/{subject}/*).
	// This would require:
	// - Reading S3 bucket/credentials from env vars
	// - Using AWS SDK to list and delete objects by prefix
	// - Counting deleted objects and returning the total
	e.logger.Debug("s3 erasure (stub)", zap.String("tenant", tenant), zap.String("subject", subjectID))
	return 0, nil
}

// MongoEraser implements the Eraser interface for MongoDB.
type MongoEraser struct {
	logger *zap.Logger
}

// NewMongoEraser creates a new MongoDB eraser.
func NewMongoEraser(logger *zap.Logger) *MongoEraser {
	return &MongoEraser{logger: logger}
}

// Erase deletes all documents for a subject from MongoDB.
// For now, a real implementation would connect to mongo and delete by query.
func (e *MongoEraser) Erase(ctx context.Context, tenant, subjectID string) (int, error) {
	// TODO: Connect to MongoDB and delete documents.
	// This would require:
	// - Reading mongodb connection string from env vars
	// - Identifying collections with subject data
	// - Executing deleteMany queries on all relevant collections
	// - Counting deleted documents and returning the total
	e.logger.Debug("mongo erasure (stub)", zap.String("tenant", tenant), zap.String("subject", subjectID))
	return 0, nil
}

// IcebergEraser implements the Eraser interface for Iceberg.
type IcebergEraser struct {
	logger *zap.Logger
}

// NewIcebergEraser creates a new Iceberg eraser.
func NewIcebergEraser(logger *zap.Logger) *IcebergEraser {
	return &IcebergEraser{logger: logger}
}

// Erase deletes all data for a subject from Iceberg.
// For now, a real implementation would delete iceberg rows by subject/tenant predicate.
func (e *IcebergEraser) Erase(ctx context.Context, tenant, subjectID string) (int, error) {
	// TODO: Connect to Iceberg catalog and delete rows.
	// This would require:
	// - Reading iceberg catalog/warehouse details from env vars
	// - Identifying tables with subject/tenant columns
	// - Executing delete queries (Iceberg delete operation)
	// - Counting deleted rows and returning the total
	e.logger.Debug("iceberg erasure (stub)", zap.String("tenant", tenant), zap.String("subject", subjectID))
	return 0, nil
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
