package store

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// DataResourceRecord is the in-memory P1 representation of a DataResource.
// In P2+ this is backed by etcd/K8s CRD state.
type DataResourceRecord struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Tenant      string    `json:"tenant"`
	Type        string    `json:"type"`
	Class       string    `json:"class"`
	Origination string    `json:"origination"`
	Phase       string    `json:"phase"`
	HA          bool      `json:"ha"`
	Protocols   []string  `json:"protocols"`
	CreatedAt   time.Time `json:"createdAt"`
}

// Store is the data access interface for the API server.
type Store interface {
	ListDataResources(ctx context.Context, tenant string) ([]*DataResourceRecord, error)
	CreateDataResource(ctx context.Context, dr *DataResourceRecord) error
	GetDataResource(ctx context.Context, tenant, name string) (*DataResourceRecord, error)
	DeleteDataResource(ctx context.Context, tenant, name string) error
	CountDataResources(ctx context.Context, tenant string) (int, error)
}

// MemoryStore is a simple in-memory store for P1 local dev.
// Production uses the K8s CRD store wired to the controller.
type MemoryStore struct {
	mu        sync.RWMutex
	resources map[string]*DataResourceRecord // key: tenant/name
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		resources: make(map[string]*DataResourceRecord),
	}
}

func key(tenant, name string) string {
	return tenant + "/" + name
}

func (s *MemoryStore) ListDataResources(ctx context.Context, tenant string) ([]*DataResourceRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*DataResourceRecord
	for _, dr := range s.resources {
		if dr.Tenant == tenant {
			result = append(result, dr)
		}
	}
	return result, nil
}

func (s *MemoryStore) CreateDataResource(ctx context.Context, dr *DataResourceRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(dr.Tenant, dr.Name)
	if _, exists := s.resources[k]; exists {
		return fmt.Errorf("DataResource %s already exists", dr.Name)
	}
	s.resources[k] = dr
	return nil
}

func (s *MemoryStore) GetDataResource(ctx context.Context, tenant, name string) (*DataResourceRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dr, ok := s.resources[key(tenant, name)]
	if !ok {
		return nil, fmt.Errorf("DataResource %s not found", name)
	}
	return dr, nil
}

func (s *MemoryStore) DeleteDataResource(ctx context.Context, tenant, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(tenant, name)
	if _, ok := s.resources[k]; !ok {
		return fmt.Errorf("DataResource %s not found", name)
	}
	delete(s.resources, k)
	return nil
}

func (s *MemoryStore) CountDataResources(ctx context.Context, tenant string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, dr := range s.resources {
		if dr.Tenant == tenant {
			count++
		}
	}
	return count, nil
}
