package pool

import (
	"testing"

	"go.uber.org/zap"
)

func TestPoolGetPutLifecycle(t *testing.T) {
	p := NewPool(2, zap.NewNop())
	p.RegisterProtocol("mysql")
	p.RegisterProtocol("mysql") // idempotent — already registered

	// Get on an unregistered protocol errors.
	if _, err := p.Get("redis"); err == nil {
		t.Error("Get on unregistered protocol should error")
	}

	// Get then Put returns the connection to the pool.
	c1, err := p.Get("mysql")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if err := p.Put("mysql", c1); err != nil {
		t.Fatalf("Put: %v", err)
	}
	// The next Get should hand back the pooled connection.
	c2, err := p.Get("mysql")
	if err != nil {
		t.Fatalf("Get after Put: %v", err)
	}
	if c2 == nil {
		t.Fatal("expected a pooled connection")
	}

	// Put on an unregistered protocol errors (and closes the conn).
	if err := p.Put("redis", c2); err == nil {
		t.Error("Put on unregistered protocol should error")
	}

	if err := p.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestPoolExhaustionSingleConn(t *testing.T) {
	p := NewPool(1, zap.NewNop())
	p.RegisterProtocol("mysql")

	if _, err := p.Get("mysql"); err != nil {
		t.Fatalf("first Get: %v", err)
	}
	// maxConns=1 already active and channel empty -> exhausted.
	if _, err := p.Get("mysql"); err == nil {
		t.Error("expected pool-exhausted error on second Get")
	}
}

func TestNewPoolNilLogger(t *testing.T) {
	if p := NewPool(5, nil); p == nil {
		t.Fatal("NewPool(nil logger) returned nil")
	}
}

// TestPoolPutToFull exercises the "pool full" branch of Put: the connection is
// closed and the active count decremented rather than buffered.
func TestPoolPutToFull(t *testing.T) {
	p := NewPool(1, zap.NewNop())
	p.RegisterProtocol("mysql")

	c1, err := p.Get("mysql")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if err := p.Put("mysql", c1); err != nil { // fills the single-slot channel
		t.Fatalf("Put: %v", err)
	}
	// A second connection returned while the channel is full -> full branch.
	extra := &mockConn{protocol: "mysql", pool: p.pools["mysql"]}
	if err := p.Put("mysql", extra); err != nil {
		t.Errorf("Put to full pool should not error: %v", err)
	}
	if !extra.closed {
		t.Error("connection returned to a full pool should be closed")
	}

	// Exercise GetStats for coverage of the stats surface.
	if stats := p.GetStats(); stats == nil {
		t.Error("GetStats returned nil")
	}
}
