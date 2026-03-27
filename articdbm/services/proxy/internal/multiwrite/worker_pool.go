package multiwrite

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

type WorkerPool struct {
	workers    []*Worker
	jobQueue   chan *Job
	resultChan chan *JobResult
	logger     *zap.Logger
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	stats      WorkerPoolStats
}

type Worker struct {
	id       int
	jobQueue chan *Job
	logger   *zap.Logger
	ctx      context.Context
}

type Job struct {
	ID       string
	Type     string
	Payload  interface{}
	Context  context.Context
	Callback func(*JobResult)
}

type JobResult struct {
	ID      string
	Success bool
	Result  interface{}
	Error   error
}

type WorkerPoolStats struct {
	TotalJobs     uint64 `json:"total_jobs"`
	CompletedJobs uint64 `json:"completed_jobs"`
	FailedJobs    uint64 `json:"failed_jobs"`
	ActiveWorkers int    `json:"active_workers"`
	QueueSize     int    `json:"queue_size"`
}

func NewWorkerPool(size int, logger *zap.Logger) (*WorkerPool, error) {
	ctx, cancel := context.WithCancel(context.Background())

	wp := &WorkerPool{
		workers:    make([]*Worker, size),
		jobQueue:   make(chan *Job, size*10), // Buffer for jobs
		resultChan: make(chan *JobResult, size*5),
		logger:     logger,
		ctx:        ctx,
		cancel:     cancel,
	}

	// Create workers
	for i := 0; i < size; i++ {
		worker := &Worker{
			id:       i,
			jobQueue: wp.jobQueue,
			logger:   logger,
			ctx:      ctx,
		}
		wp.workers[i] = worker

		wp.wg.Add(1)
		go wp.startWorker(worker)
	}

	wp.stats.ActiveWorkers = size

	// Start result processor
	go wp.processResults()

	logger.Info("Worker pool started", zap.Int("workers", size))
	return wp, nil
}

func (wp *WorkerPool) startWorker(worker *Worker) {
	defer wp.wg.Done()

	// Pin worker to specific CPU for NUMA optimization
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	wp.logger.Debug("Worker started", zap.Int("worker_id", worker.id))

	for {
		select {
		case <-wp.ctx.Done():
			return
		case job := <-wp.jobQueue:
			wp.processJob(worker, job)
		}
	}
}

func (wp *WorkerPool) processJob(worker *Worker, job *Job) {
	atomic.AddUint64(&wp.stats.TotalJobs, 1)

	result := &JobResult{
		ID: job.ID,
	}

	defer func() {
		if r := recover(); r != nil {
			result.Success = false
			result.Error = fmt.Errorf("job panicked: %v", r)
			wp.logger.Error("Job panic",
				zap.Int("worker_id", worker.id),
				zap.String("job_id", job.ID),
				zap.Any("panic", r))
		}

		// Send result
		select {
		case wp.resultChan <- result:
		case <-wp.ctx.Done():
		}
	}()

	// Execute job based on type
	switch job.Type {
	case "write":
		wp.processWriteJob(worker, job, result)
	case "health_check":
		wp.processHealthCheckJob(worker, job, result)
	case "cache_invalidate":
		wp.processCacheInvalidateJob(worker, job, result)
	default:
		result.Success = false
		result.Error = fmt.Errorf("unknown job type: %s", job.Type)
	}
}

func (wp *WorkerPool) processWriteJob(worker *Worker, job *Job, result *JobResult) {
	writeReq, ok := job.Payload.(*WriteJobPayload)
	if !ok {
		result.Success = false
		result.Error = fmt.Errorf("invalid write job payload")
		return
	}

	// Execute the write operation
	clusterResult, err := wp.executeWrite(job.Context, writeReq)
	if err != nil {
		result.Success = false
		result.Error = err
	} else {
		result.Success = clusterResult.Error == nil
		result.Result = clusterResult
	}
}

func (wp *WorkerPool) processHealthCheckJob(worker *Worker, job *Job, result *JobResult) {
	healthReq, ok := job.Payload.(*HealthCheckPayload)
	if !ok {
		result.Success = false
		result.Error = fmt.Errorf("invalid health check payload")
		return
	}

	// Execute health check
	healthy, err := wp.executeHealthCheck(job.Context, healthReq)
	result.Success = err == nil
	result.Result = healthy
	result.Error = err
}

func (wp *WorkerPool) processCacheInvalidateJob(worker *Worker, job *Job, result *JobResult) {
	cacheReq, ok := job.Payload.(*CacheInvalidatePayload)
	if !ok {
		result.Success = false
		result.Error = fmt.Errorf("invalid cache invalidate payload")
		return
	}

	// Execute cache invalidation
	err := wp.executeCacheInvalidate(job.Context, cacheReq)
	result.Success = err == nil
	result.Error = err
}

func (wp *WorkerPool) processResults() {
	for {
		select {
		case <-wp.ctx.Done():
			return
		case result := <-wp.resultChan:
			if result.Success {
				atomic.AddUint64(&wp.stats.CompletedJobs, 1)
			} else {
				atomic.AddUint64(&wp.stats.FailedJobs, 1)
			}

			wp.logger.Debug("Job completed",
				zap.String("job_id", result.ID),
				zap.Bool("success", result.Success))
		}
	}
}

// Job payload structures
type WriteJobPayload struct {
	Cluster *ClusterConfig
	Request *WriteRequest
}

type HealthCheckPayload struct {
	Cluster *ClusterConfig
	Query   string
}

type CacheInvalidatePayload struct {
	CacheKeys []string
	Pattern   string
}

func (wp *WorkerPool) executeWrite(ctx context.Context, payload *WriteJobPayload) (*ClusterResult, error) {
	// This would integrate with the actual database drivers
	// For now, simulate the write operation

	result := &ClusterResult{
		ClusterName: payload.Cluster.Name,
		Status:      StatusSuccess,
		IsPrimary:   payload.Cluster.Priority == 1,
	}

	// Simulate write execution time
	select {
	case <-ctx.Done():
		result.Status = StatusTimeout
		result.Error = ctx.Err()
		return result, ctx.Err()
	case <-time.After(time.Duration(wp.simulateWriteLatency()) * time.Millisecond):
		result.RowsAffected = 1
		return result, nil
	}
}

func (wp *WorkerPool) executeHealthCheck(ctx context.Context, payload *HealthCheckPayload) (bool, error) {
	// Simulate health check
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-time.After(5 * time.Millisecond):
		// Simulate 95% uptime
		return wp.simulateHealthCheck(), nil
	}
}

func (wp *WorkerPool) executeCacheInvalidate(ctx context.Context, payload *CacheInvalidatePayload) error {
	// Simulate cache invalidation
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(1 * time.Millisecond):
		return nil
	}
}

// Simulation helpers (replace with real implementations)
func (wp *WorkerPool) simulateWriteLatency() int {
	// Simulate 1-50ms write latency
	return 1 + int(atomic.LoadUint64(&wp.stats.TotalJobs))%50
}

func (wp *WorkerPool) simulateHealthCheck() bool {
	// Simulate 95% healthy rate
	return atomic.LoadUint64(&wp.stats.TotalJobs)%20 != 0
}

func (wp *WorkerPool) SubmitJob(job *Job) error {
	select {
	case wp.jobQueue <- job:
		return nil
	case <-wp.ctx.Done():
		return wp.ctx.Err()
	default:
		return fmt.Errorf("job queue full")
	}
}

func (wp *WorkerPool) GetStats() WorkerPoolStats {
	wp.stats.QueueSize = len(wp.jobQueue)
	return wp.stats
}

func (wp *WorkerPool) Close() error {
	wp.cancel()

	// Wait for all workers to finish
	wp.wg.Wait()

	wp.logger.Info("Worker pool stopped")
	return nil
}