package pool

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestPooledConnInterface exercises the net.Conn surface of the pooled
// connection returned by Pool.Get, including the after-close error paths.
func TestPooledConnInterface(t *testing.T) {
	p := NewPool(10, zap.NewNop())
	p.RegisterProtocol("mysql")

	conn, err := p.Get("mysql")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if n, err := conn.Write([]byte("hello")); err != nil || n != 5 {
		t.Errorf("Write = (%d, %v), want (5, nil)", n, err)
	}
	if n, err := conn.Read(make([]byte, 4)); err != nil || n != 0 {
		t.Errorf("Read = (%d, %v), want (0, nil)", n, err)
	}
	if conn.LocalAddr() == nil || conn.RemoteAddr() == nil {
		t.Error("Local/RemoteAddr must be non-nil")
	}
	for _, err := range []error{
		conn.SetDeadline(time.Time{}),
		conn.SetReadDeadline(time.Time{}),
		conn.SetWriteDeadline(time.Time{}),
	} {
		if err != nil {
			t.Errorf("deadline setter err = %v", err)
		}
	}

	if err := conn.Close(); err != nil {
		t.Errorf("Close err = %v", err)
	}
	// After close, I/O must error.
	if _, err := conn.Write([]byte("x")); err == nil {
		t.Error("Write after Close should error")
	}
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Error("Read after Close should error")
	}
}
