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
	s.mu.Unlock()

	now := time.Now()
	s.mu.Lock()
	// P2: real backend erasure not yet implemented.
	// Fail loudly with "failed" status and error message to prevent callers
	// from believing data was erased when it wasn't (GDPR compliance issue).
	req.Status = "failed"
	req.Error = "erasure execution not implemented; P2 feature"
	req.CompletedAt = &now
	// Clear progress to reflect the failure
	req.Progress = make(map[string]string)
	s.mu.Unlock()

	s.logger.Info("erasure request failed (real erasure P2)",
		zap.String("id", req.ID),
		zap.String("subject", req.SubjectID),
		zap.String("status", "failed"),
		zap.String("error", "erasure execution not implemented"),
	)
}
