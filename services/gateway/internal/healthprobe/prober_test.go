package healthprobe

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNewProber(t *testing.T) {
	logger := zap.NewNop()
	handler := func(r HealthResult) {}

	p := NewProber(30*time.Second, handler, logger)
	if p == nil {
		t.Error("NewProber() returned nil")
	}
}

func TestProber_Register(t *testing.T) {
	logger := zap.NewNop()
	handler := func(r HealthResult) {}
	p := NewProber(30*time.Second, handler, logger)

	target := ResourceTarget{
		Tenant:      "tenant1",
		Name:        "resource1",
		Origination: "imported",
		Endpoint:    "localhost:5432",
	}

	p.Register(target)

	// Verify the target is registered by checking the internal state
	p.mu.RLock()
	_, ok := p.targets["tenant1/resource1"]
	p.mu.RUnlock()

	if !ok {
		t.Error("Register() failed to add target")
	}
}

func TestProber_Unregister(t *testing.T) {
	logger := zap.NewNop()
	handler := func(r HealthResult) {}
	p := NewProber(30*time.Second, handler, logger)

	target := ResourceTarget{
		Tenant:      "tenant1",
		Name:        "resource1",
		Origination: "imported",
		Endpoint:    "localhost:5432",
	}

	p.Register(target)
	p.Unregister("tenant1", "resource1")

	p.mu.RLock()
	_, ok := p.targets["tenant1/resource1"]
	p.mu.RUnlock()

	if ok {
		t.Error("Unregister() failed to remove target")
	}
}

func TestProber_Run_ContextCancellation(t *testing.T) {
	logger := zap.NewNop()
	callCount := 0
	handler := func(r HealthResult) {
		callCount++
	}

	p := NewProber(10*time.Millisecond, handler, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run prober for a short time then cancel
	done := make(chan bool)
	go func() {
		p.Run(ctx)
		done <- true
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Success - prober exited
	case <-time.After(1 * time.Second):
		t.Error("Run() did not exit after context cancellation")
	}
}

func TestProber_Register_UpdateExisting(t *testing.T) {
	logger := zap.NewNop()
	handler := func(r HealthResult) {}
	p := NewProber(30*time.Second, handler, logger)

	target1 := ResourceTarget{
		Tenant:      "tenant1",
		Name:        "resource1",
		Origination: "imported",
		Endpoint:    "localhost:5432",
	}

	target2 := ResourceTarget{
		Tenant:      "tenant1",
		Name:        "resource1",
		Origination: "external",
		Endpoint:    "example.com:5432",
	}

	p.Register(target1)
	p.Register(target2)

	// Verify the target is updated
	p.mu.RLock()
	stored := p.targets["tenant1/resource1"]
	p.mu.RUnlock()

	if stored.Origination != "external" {
		t.Errorf("Register() should update existing target, got origination %q", stored.Origination)
	}
}

func TestProber_MultipleTargets(t *testing.T) {
	logger := zap.NewNop()
	handler := func(r HealthResult) {}
	p := NewProber(30*time.Second, handler, logger)

	targets := []ResourceTarget{
		{Tenant: "tenant1", Name: "res1", Origination: "imported", Endpoint: "host1:5432"},
		{Tenant: "tenant1", Name: "res2", Origination: "imported", Endpoint: "host2:5432"},
		{Tenant: "tenant2", Name: "res3", Origination: "external", Endpoint: "host3:5432"},
	}

	for _, target := range targets {
		p.Register(target)
	}

	p.mu.RLock()
	count := len(p.targets)
	p.mu.RUnlock()

	if count != 3 {
		t.Errorf("Register() stored %d targets, want 3", count)
	}
}

func TestProber_Fields(t *testing.T) {
	logger := zap.NewNop()
	handler := func(r HealthResult) {}
	interval := 45 * time.Second

	p := NewProber(interval, handler, logger)

	if p.interval != interval {
		t.Errorf("Prober.interval = %v, want %v", p.interval, interval)
	}

	if p.handler == nil {
		t.Error("Prober.handler should be set")
	}

	if p.log != logger {
		t.Error("Prober.log should be set to provided logger")
	}
}

func TestResourceTarget_Structure(t *testing.T) {
	target := ResourceTarget{
		Tenant:      "tenant1",
		Name:        "resource1",
		Origination: "imported",
		Endpoint:    "localhost:5432",
		HTTPProbe:   "http://localhost:5432/health",
	}

	if target.Tenant != "tenant1" {
		t.Errorf("Tenant = %q, want tenant1", target.Tenant)
	}
	if target.Name != "resource1" {
		t.Errorf("Name = %q, want resource1", target.Name)
	}
	if target.Origination != "imported" {
		t.Errorf("Origination = %q, want imported", target.Origination)
	}
	if target.Endpoint != "localhost:5432" {
		t.Errorf("Endpoint = %q, want localhost:5432", target.Endpoint)
	}
	if target.HTTPProbe != "http://localhost:5432/health" {
		t.Errorf("HTTPProbe = %q, want http://localhost:5432/health", target.HTTPProbe)
	}
}

func TestHealthResult_Structure(t *testing.T) {
	now := time.Now()
	result := HealthResult{
		Tenant:   "tenant1",
		Name:     "resource1",
		State:    "healthy",
		Message:  "OK",
		ProbedAt: now,
	}

	if result.Tenant != "tenant1" {
		t.Errorf("Tenant = %q, want tenant1", result.Tenant)
	}
	if result.Name != "resource1" {
		t.Errorf("Name = %q, want resource1", result.Name)
	}
	if result.State != "healthy" {
		t.Errorf("State = %q, want healthy", result.State)
	}
	if result.Message != "OK" {
		t.Errorf("Message = %q, want OK", result.Message)
	}
	if !result.ProbedAt.Equal(now) {
		t.Errorf("ProbedAt should match provided time")
	}
}
