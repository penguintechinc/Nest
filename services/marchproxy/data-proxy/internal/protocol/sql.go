package protocol

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

func init() { Register(&sqlProtocol{}) }

type sqlProtocol struct{}

func (p *sqlProtocol) Name() string { return "sql" }
func (p *sqlProtocol) Protocols() []string {
	return []string{"postgresql", "postgres", "mysql", "mariadb"}
}

func (p *sqlProtocol) Dial(ctx context.Context, cfg ProtocolConfig) (BackendConn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("sql dial %s: %w", cfg.Endpoint, err)
	}
	return &netConn{conn: conn}, nil
}

func (p *sqlProtocol) HealthCheck(ctx context.Context, cfg ProtocolConfig) (*HealthResult, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", cfg.Endpoint)
	if err != nil {
		return &HealthResult{State: "failed", Message: fmt.Sprintf("tcp connect failed: %v", err)}, nil
	}
	conn.Close()

	role := cfg.Extra["role"]
	if role == "" {
		role = "primary"
	}

	return &HealthResult{
		State:   "healthy",
		Message: fmt.Sprintf("sql endpoint reachable (role=%s)", role),
	}, nil
}

func IsReadQuery(sql string) bool {
	trimmed := strings.TrimSpace(strings.ToUpper(sql))
	if strings.Contains(sql, "/*+ nest_primary */") {
		return false
	}
	return strings.HasPrefix(trimmed, "SELECT") ||
		strings.HasPrefix(trimmed, "SHOW") ||
		strings.HasPrefix(trimmed, "EXPLAIN") ||
		strings.HasPrefix(trimmed, "DESCRIBE")
}

func (p *sqlProtocol) PoolConfig(cfg ProtocolConfig) PoolOptions {
	return PoolOptions{MaxConns: 20, MaxIdleTime: 300}
}

type netConn struct{ conn net.Conn }

func (c *netConn) Close() error { return c.conn.Close() }

// HedgedDial sends the connection attempt to two endpoints simultaneously
// and returns the first successful connection, cancelling the other.
// endpoints should be [primary, replica]; if only one given, falls back to Dial.
func (p *sqlProtocol) HedgedDial(ctx context.Context, cfgs []ProtocolConfig) (BackendConn, error) {
	if len(cfgs) < 2 {
		if len(cfgs) == 0 {
			return nil, fmt.Errorf("no endpoints provided for hedged dial")
		}
		return p.Dial(ctx, cfgs[0])
	}

	type result struct {
		conn BackendConn
		err  error
	}
	ch := make(chan result, 2)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, cfg := range cfgs[:2] {
		cfg := cfg
		go func() {
			conn, err := p.Dial(ctx, cfg)
			ch <- result{conn, err}
		}()
	}

	var firstErr error
	for i := 0; i < 2; i++ {
		r := <-ch
		if r.err == nil {
			cancel() // cancel the other goroutine
			return r.conn, nil
		}
		firstErr = r.err
	}
	return nil, firstErr
}
