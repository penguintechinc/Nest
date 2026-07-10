package routing

import (
	"context"
	"net"
	"testing"
	"time"
)

// liveAddr returns a listening TCP listener and its host/port (healthy backend).
func liveAddr(t *testing.T) (net.Listener, string, int) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	a := lis.Addr().(*net.TCPAddr)
	return lis, "127.0.0.1", a.Port
}

// deadPort returns a host/port that nothing is listening on (unhealthy backend).
func deadPort(t *testing.T) (string, int) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	p := lis.Addr().(*net.TCPAddr).Port
	_ = lis.Close() // free it → dials get connection-refused
	return "127.0.0.1", p
}

func TestCheckBackendHealth_UpAndDown(t *testing.T) {
	r := NewRouter(nil)

	lis, host, port := liveAddr(t)
	defer lis.Close()
	up := &BackendEndpoint{Name: "up", Host: host, Port: port}
	r.checkBackendHealth(up)
	if !up.Healthy.Load() {
		t.Error("live backend should be marked healthy")
	}
	if up.FailCount.Load() != 0 {
		t.Errorf("healthy backend FailCount = %d, want 0", up.FailCount.Load())
	}

	dh, dp := deadPort(t)
	down := &BackendEndpoint{Name: "down", Host: dh, Port: dp}
	r.checkBackendHealth(down)
	if down.Healthy.Load() {
		t.Error("dead backend should be marked unhealthy")
	}
	if down.FailCount.Load() != 1 {
		t.Errorf("unhealthy backend FailCount = %d, want 1", down.FailCount.Load())
	}
}

func TestHealthChecks_StartStop(t *testing.T) {
	r := NewRouter(nil)
	r.healthCheckInterval = 10 * time.Millisecond

	lis, host, port := liveAddr(t)
	defer lis.Close()
	dh, dp := deadPort(t)

	route := &RouteConfig{
		ID:       "r1",
		Protocol: "postgresql",
		Primary:  &BackendEndpoint{Name: "primary", Host: host, Port: port},
		Replicas: []*BackendEndpoint{{Name: "replica-1", Host: dh, Port: dp}},
	}
	if err := r.AddRoute(route); err != nil {
		t.Fatalf("AddRoute: %v", err)
	}

	r.StartHealthChecks(context.Background())
	// Wait for at least one tick to run checkAllBackends -> checkBackendHealth.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if route.Primary.Healthy.Load() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	r.StopHealthChecks()

	if !route.Primary.Healthy.Load() {
		t.Error("primary should be healthy after health checks")
	}
	if route.Replicas[0].Healthy.Load() {
		t.Error("dead replica should be unhealthy after health checks")
	}
}

func TestHealthChecks_ContextCancel(t *testing.T) {
	r := NewRouter(nil)
	r.healthCheckInterval = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	r.StartHealthChecks(ctx)
	// Cancelling the context must terminate the goroutine (covers ctx.Done()).
	cancel()
	r.healthCheckTicker.Stop()
}

func TestGetActivePrimary_BlueGreen(t *testing.T) {
	blue := &BackendEndpoint{Name: "blue", Host: "b", Port: 1}
	green := &BackendEndpoint{Name: "green", Host: "g", Port: 2}
	legacy := &BackendEndpoint{Name: "legacy", Host: "l", Port: 3}

	// active=blue
	rc := &RouteConfig{Primary: legacy, BlueGreen: &BlueGreenConfig{Blue: blue, Green: green, Active: "blue"}}
	if got := rc.GetActivePrimary(); got != blue {
		t.Errorf("active blue: got %v", got)
	}
	// active=green
	if err := rc.SetActiveColor("green"); err != nil {
		t.Fatalf("SetActiveColor: %v", err)
	}
	if got := rc.GetActivePrimary(); got != green {
		t.Errorf("active green: got %v", got)
	}
	// blue/green configured but active color endpoint nil -> falls back to legacy primary
	rc2 := &RouteConfig{Primary: legacy, BlueGreen: &BlueGreenConfig{Active: "blue"}}
	if got := rc2.GetActivePrimary(); got != legacy {
		t.Errorf("nil active endpoint should fall back to legacy primary, got %v", got)
	}
	// no blue/green -> legacy primary
	rc3 := &RouteConfig{Primary: legacy}
	if got := rc3.GetActivePrimary(); got != legacy {
		t.Errorf("no blue/green: got %v", got)
	}
}

func TestSetActiveColor_Errors(t *testing.T) {
	// no blue/green configured
	rc := &RouteConfig{Primary: &BackendEndpoint{}}
	if err := rc.SetActiveColor("blue"); err == nil {
		t.Error("expected error when blue/green not configured")
	}
	// invalid color
	rc.BlueGreen = &BlueGreenConfig{Active: "blue"}
	if err := rc.SetActiveColor("purple"); err == nil {
		t.Error("expected error for invalid color")
	}
	if err := rc.SetActiveColor("green"); err != nil {
		t.Errorf("valid color should succeed: %v", err)
	}
}

func TestGetWriteTargets(t *testing.T) {
	primary := &BackendEndpoint{Name: "primary"}
	a := &BackendEndpoint{Name: "a"}
	b := &BackendEndpoint{Name: "b"}

	// multi-write enabled -> all targets
	rc := &RouteConfig{
		Primary: primary,
		MultiWrite: &MultiWriteConfig{
			Enabled: true,
			Targets: []*MultiWriteTarget{{Endpoint: a}, {Endpoint: b, Authoritative: true}},
		},
	}
	got := rc.GetWriteTargets()
	if len(got) != 2 || got[0] != a || got[1] != b {
		t.Errorf("multi-write targets = %v, want [a b]", got)
	}

	// no multi-write -> [active primary]
	rc2 := &RouteConfig{Primary: primary}
	got2 := rc2.GetWriteTargets()
	if len(got2) != 1 || got2[0] != primary {
		t.Errorf("single write target = %v, want [primary]", got2)
	}

	// nil primary, no multi-write -> nil
	rc3 := &RouteConfig{}
	if got3 := rc3.GetWriteTargets(); got3 != nil {
		t.Errorf("nil primary should yield nil write targets, got %v", got3)
	}
}

func TestGetAuthoritativeWriteTarget(t *testing.T) {
	primary := &BackendEndpoint{Name: "primary"}
	a := &BackendEndpoint{Name: "a"}
	b := &BackendEndpoint{Name: "b"}

	// explicit authoritative target
	rc := &RouteConfig{
		Primary:    primary,
		MultiWrite: &MultiWriteConfig{Enabled: true, Targets: []*MultiWriteTarget{{Endpoint: a}, {Endpoint: b, Authoritative: true}}},
	}
	if got := rc.GetAuthoritativeWriteTarget(); got != b {
		t.Errorf("authoritative = %v, want b", got)
	}

	// no explicit authoritative -> first target
	rc2 := &RouteConfig{
		Primary:    primary,
		MultiWrite: &MultiWriteConfig{Enabled: true, Targets: []*MultiWriteTarget{{Endpoint: a}, {Endpoint: b}}},
	}
	if got := rc2.GetAuthoritativeWriteTarget(); got != a {
		t.Errorf("first target = %v, want a", got)
	}

	// no multi-write -> active primary
	rc3 := &RouteConfig{Primary: primary}
	if got := rc3.GetAuthoritativeWriteTarget(); got != primary {
		t.Errorf("fallback = %v, want primary", got)
	}
}
