package protocol

import (
	"context"
	"fmt"
	"sync"
)

// BackendProtocol handles wire-protocol connectivity to a specific backend type.
type BackendProtocol interface {
	Name() string
	Protocols() []string
	Dial(ctx context.Context, cfg ProtocolConfig) (BackendConn, error)
	HealthCheck(ctx context.Context, cfg ProtocolConfig) (*HealthResult, error)
	PoolConfig(cfg ProtocolConfig) PoolOptions
}

// BackendConn represents an active connection to a backend.
type BackendConn interface {
	Close() error
}

// ProtocolConfig holds connection parameters for a backend.
type ProtocolConfig struct {
	Endpoint    string
	AuthType    string
	Credential  string
	TLSRequired bool
	Extra       map[string]string
}

// HealthResult reports the health status of a backend.
type HealthResult struct {
	State   string
	Message string
}

// PoolOptions controls connection pool behavior.
type PoolOptions struct {
	MaxConns    int
	MaxIdleTime int // seconds
}

var (
	mu        sync.RWMutex
	protocols = map[string]BackendProtocol{}
)

// Register adds a BackendProtocol to the global registry.
func Register(p BackendProtocol) {
	mu.Lock()
	defer mu.Unlock()
	for _, proto := range p.Protocols() {
		protocols[proto] = p
	}
}

// Get returns the BackendProtocol registered for the given protocol name.
func Get(name string) (BackendProtocol, error) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := protocols[name]
	if !ok {
		return nil, fmt.Errorf("no protocol handler registered for %q", name)
	}
	return p, nil
}

// All returns all registered protocol handlers.
func All() []BackendProtocol {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]BackendProtocol, 0, len(protocols))
	seen := map[string]bool{}
	for _, p := range protocols {
		if !seen[p.Name()] {
			out = append(out, p)
			seen[p.Name()] = true
		}
	}
	return out
}
