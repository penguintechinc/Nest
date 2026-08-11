package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func TestNewRedisStore_NilLogger(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	// nil logger -> replaced with a no-op logger internally.
	if rs := NewRedisStore(client, "p", time.Second, 0, nil); rs == nil {
		t.Fatal("NewRedisStore returned nil")
	}
}

func TestDecodeFrames_Errors(t *testing.T) {
	// Empty data -> frame-count read fails.
	if _, err := decodeFrames(nil); err == nil {
		t.Error("expected error decoding empty data")
	}
	// Frame count = 1 but no length bytes follow.
	if _, err := decodeFrames([]byte{0x00, 0x00, 0x00, 0x01}); err == nil {
		t.Error("expected error decoding truncated frame length")
	}
	// Frame count = 1, length = 10, but no data -> frame-data read fails.
	truncated := []byte{0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x0A}
	if _, err := decodeFrames(truncated); err == nil {
		t.Error("expected error decoding truncated frame data")
	}
}

func TestRedisStore_Get_DecodeError(t *testing.T) {
	rs, mr := newTestStore(t)
	defer mr.Close()
	ctx := context.Background()

	// Plant undecodable bytes directly at the computed key.
	key := rs.makeKey("t1", "db", "select 1")
	if err := mr.Set(key, "not-a-valid-frame-blob"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := rs.Get(ctx, "t1", "db", "select 1"); err == nil {
		t.Error("expected decode error on Get")
	}
}

func TestRedisStore_Get_ClientError(t *testing.T) {
	rs, mr := newTestStore(t)
	mr.Close() // sever the connection -> client op errors
	if _, err := rs.Get(context.Background(), "t", "db", "q"); err == nil {
		t.Error("expected client error on Get after close")
	}
}

func TestRedisStore_Set_SizeLimitSkip(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	rs := NewRedisStore(client, "p", time.Second, 1, zap.NewNop()) // 1KB cap

	big := [][]byte{make([]byte, 2048)} // 2KB > cap -> silently skipped
	if err := rs.Set(context.Background(), "t", "db", "q", big, time.Second, nil); err != nil {
		t.Errorf("Set over size cap should skip, not error: %v", err)
	}
	// Nothing should have been stored.
	if v, _ := rs.Get(context.Background(), "t", "db", "q"); v != nil {
		t.Error("oversized entry should not be cached")
	}
}

func TestRedisStore_Set_ClientError(t *testing.T) {
	rs, mr := newTestStore(t)
	mr.Close()
	err := rs.Set(context.Background(), "t", "db", "q", [][]byte{{0x01}}, time.Second, []string{"users"})
	if err == nil {
		t.Error("expected client error on Set after close")
	}
}

func TestRedisStore_InvalidateByTable_NoMembers(t *testing.T) {
	rs, mr := newTestStore(t)
	defer mr.Close()
	// No tags exist for this table -> empty member set -> nil.
	if err := rs.InvalidateByTable(context.Background(), "never_tagged"); err != nil {
		t.Errorf("InvalidateByTable(no members) = %v, want nil", err)
	}
}

func TestRedisStore_InvalidateByTable_ClientError(t *testing.T) {
	rs, mr := newTestStore(t)
	mr.Close()
	if err := rs.InvalidateByTable(context.Background(), "users"); err == nil {
		t.Error("expected client error on InvalidateByTable after close")
	}
}
