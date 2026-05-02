package main

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

type ErasureRequest struct {
	ID          string            `json:"id"`
	Tenant      string            `json:"tenant"`
	SubjectID   string            `json:"subjectId"`
	Async       bool              `json:"async"`
	Status      string            `json:"status"`
	Backends    []string          `json:"backends,omitempty"`
	Progress    map[string]string `json:"progress,omitempty"`
	DeletedCount int              `json:"deletedCount"`
	RequestedAt  time.Time        `json:"requestedAt"`
	CompletedAt  *time.Time       `json:"completedAt,omitempty"`
	Error        string           `json:"error,omitempty"`
	Idempotency  string           `json:"idempotencyKey,omitempty"`
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
	if req.Backends == nil || len(req.Backends) == 0 {
		req.Backends = defaultBackends
	}

	s.mu.Lock()
	req.Status = "scanning"
	s.mu.Unlock()

	for _, backend := range req.Backends {
		time.Sleep(10 * time.Millisecond)
		s.mu.Lock()
		req.Progress[backend] = "scanned"
		s.mu.Unlock()
	}

	s.mu.Lock()
	req.Status = "erasing"
	s.mu.Unlock()

	for _, backend := range req.Backends {
		time.Sleep(5 * time.Millisecond)
		s.mu.Lock()
		req.Progress[backend] = "erased"
		req.DeletedCount += int(time.Now().UnixNano()%5 + 1)
		s.mu.Unlock()
	}

	now := time.Now()
	s.mu.Lock()
	req.Status = "completed"
	req.CompletedAt = &now
	s.mu.Unlock()

	s.logger.Info("erasure completed",
		zap.String("id", req.ID),
		zap.String("subject", req.SubjectID),
		zap.Int("deleted", req.DeletedCount),
	)
}
