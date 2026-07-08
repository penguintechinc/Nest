package main

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

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

type ErasureStore struct {
	mu            sync.RWMutex
	requests      map[string]*ErasureRequest
	byIdempotency map[string]string
	logger        *zap.Logger
}

func NewErasureStore(logger *zap.Logger) *ErasureStore {
	return &ErasureStore{
		requests:      make(map[string]*ErasureRequest),
		byIdempotency: make(map[string]string),
		logger:        logger,
	}
}

func (s *ErasureStore) CreateRequest(r *ErasureRequest) (*ErasureRequest, error) {
	s.mu.Lock()

	if r.Idempotency != "" {
		if id, ok := s.byIdempotency[r.Idempotency]; ok {
			existing := s.requests[id]
			s.mu.Unlock()
			return existing, nil
		}
	}

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

	s.requests[req.ID] = req
	if req.Idempotency != "" {
		s.byIdempotency[req.Idempotency] = req.ID
	}

	s.mu.Unlock()

	if r.Async {
		go s.simulateErasure(req)
	} else {
		s.simulateErasure(req)
	}

	return req, nil
}

func (s *ErasureStore) GetRequest(id string) (*ErasureRequest, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	req, ok := s.requests[id]
	return req, ok
}

func (s *ErasureStore) ListRequests(tenant string) []*ErasureRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*ErasureRequest
	for _, req := range s.requests {
		if req.Tenant == tenant {
			result = append(result, req)
		}
	}
	return result
}

func (s *ErasureStore) simulateErasure(req *ErasureRequest) {
	defaultBackends := []string{"postgres", "kafka", "s3", "mongo", "iceberg"}

	// Copy backends under lock to avoid races
	s.mu.Lock()
	if req.Backends == nil || len(req.Backends) == 0 {
		req.Backends = defaultBackends
	}
	backends := make([]string, len(req.Backends))
	copy(backends, req.Backends)
	req.Status = "scanning"
	s.mu.Unlock()

	for _, backend := range backends {
		time.Sleep(10 * time.Millisecond)
		s.mu.Lock()
		req.Progress[backend] = "scanned"
		s.mu.Unlock()
	}

	s.mu.Lock()
	req.Status = "erasing"
	s.mu.Unlock()

	// P2: real backend erasure not yet implemented.
	// For now, just simulate progress; mark status as "pending" instead of "completed"
	// to indicate the operation is not yet executed.
	for _, backend := range backends {
		time.Sleep(5 * time.Millisecond)
		s.mu.Lock()
		req.Progress[backend] = "pending"
		// Don't increment DeletedCount since erasure is not actually happening
		s.mu.Unlock()
	}

	now := time.Now()
	s.mu.Lock()
	// Mark as "pending" instead of "completed" to indicate real erasure is not yet implemented
	req.Status = "pending"
	req.CompletedAt = &now
	s.mu.Unlock()

	s.logger.Info("erasure request pending (real erasure P2)",
		zap.String("id", req.ID),
		zap.String("subject", req.SubjectID),
		zap.String("status", "pending"),
	)
}
