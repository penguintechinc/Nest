package healthprobe

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ResourceTarget is the minimal info needed to probe a resource.
type ResourceTarget struct {
	Tenant      string
	Name        string
	Origination string // "imported" or "external"
	Endpoint    string // host:port to TCP-probe
	HTTPProbe   string // optional HTTP URL for /health endpoint
}

// HealthResult is the outcome of a single probe cycle.
type HealthResult struct {
	Tenant   string
	Name     string
	State    string // healthy, degraded, unreachable
	Message  string
	ProbedAt time.Time
}

// ResultHandler is called with each probe result.
type ResultHandler func(result HealthResult)

// Prober runs health checks on a set of imported/external resources.
type Prober struct {
	mu       sync.RWMutex
	targets  map[string]ResourceTarget // key: tenant/name
	handler  ResultHandler
	interval time.Duration
	log      *zap.Logger
}

// NewProber creates a new Prober with the given check interval.
func NewProber(interval time.Duration, handler ResultHandler, log *zap.Logger) *Prober {
	return &Prober{
		targets:  make(map[string]ResourceTarget),
		handler:  handler,
		interval: interval,
		log:      log,
	}
}

// Register adds or updates a resource target.
func (p *Prober) Register(t ResourceTarget) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.targets[t.Tenant+"/"+t.Name] = t
}

// Unregister removes a resource target.
func (p *Prober) Unregister(tenant, name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.targets, tenant+"/"+name)
}

// Run starts the probe loop. It returns when ctx is cancelled.
func (p *Prober) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.probeAll(ctx)
		}
	}
}

func (p *Prober) probeAll(ctx context.Context) {
	p.mu.RLock()
	targets := make([]ResourceTarget, 0, len(p.targets))
	for _, t := range p.targets {
		targets = append(targets, t)
	}
	p.mu.RUnlock()

	var wg sync.WaitGroup
	for _, t := range targets {
		wg.Add(1)
		go func(target ResourceTarget) {
			defer wg.Done()
			result := p.probe(ctx, target)
			if p.handler != nil {
				p.handler(result)
			}
		}(t)
	}
	wg.Wait()
}

func (p *Prober) probe(ctx context.Context, t ResourceTarget) HealthResult {
	result := HealthResult{
		Tenant:   t.Tenant,
		Name:     t.Name,
		ProbedAt: time.Now().UTC(),
	}

	// Try HTTP probe first if available
	if t.HTTPProbe != "" {
		if httpReachable(ctx, t.HTTPProbe) {
			result.State = "healthy"
			return result
		}
	}

	// Fall back to TCP probe
	if t.Endpoint != "" {
		if tcpReachable(t.Endpoint, 3*time.Second) {
			result.State = "healthy"
			return result
		}
		result.State = "unreachable"
		result.Message = "TCP probe to " + t.Endpoint + " failed"
		return result
	}

	result.State = "degraded"
	result.Message = "no probe endpoint configured"
	return result
}

func tcpReachable(hostport string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", hostport, timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func httpReachable(ctx context.Context, url string) bool {
	reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}
