package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// TestNoOpStore verifies the disabled-cache store is inert but well-formed.
func TestNoOpStore(t *testing.T) {
	var s Store = &NoOpStore{}
	ctx := context.Background()

	frames, err := s.Get(ctx, "t", "db", "select 1")
	if frames != nil || err != nil {
		t.Errorf("NoOp Get = (%v, %v), want (nil, nil)", frames, err)
	}
	if err := s.Set(ctx, "t", "db", "select 1", [][]byte{{0x01}}, time.Second, []string{"users"}); err != nil {
		t.Errorf("NoOp Set err = %v", err)
	}
	if err := s.InvalidateByTable(ctx, "users"); err != nil {
		t.Errorf("NoOp InvalidateByTable err = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("NoOp Close err = %v", err)
	}
	stats := s.Stats()
	for _, k := range []string{"hits", "misses", "errors", "hit_rate", "total"} {
		if _, ok := stats[k]; !ok {
			t.Errorf("NoOp Stats missing key %q", k)
		}
	}
}

// TestRecordError exercises the error counter via the Stats surface.
func TestRecordError(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	rs := NewRedisStore(client, "test:cache", time.Second, 1000, zap.NewNop())

	rs.recordError()
	rs.recordError()
	if got := rs.Stats()["errors"]; got != int64(2) && got != 2 {
		t.Errorf("errors stat = %v, want 2", got)
	}
}
