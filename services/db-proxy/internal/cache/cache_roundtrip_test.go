package cache

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func newTestStore(t *testing.T) (*RedisStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return NewRedisStore(client, "test:cache", 10*time.Second, 1000, zap.NewNop()), mr
}

func TestCacheRoundTripAndInvalidate(t *testing.T) {
	rs, mr := newTestStore(t)
	defer mr.Close()
	ctx := context.Background()

	frames := [][]byte{{0x01, 0x02}, {0x03}}
	if err := rs.Set(ctx, "t1", "db", "select * from users", frames, 10*time.Second, []string{"users"}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Hit.
	got, err := rs.Get(ctx, "t1", "db", "select * from users")
	if err != nil {
		t.Fatalf("Get hit: %v", err)
	}
	if len(got) != 2 || !bytes.Equal(got[0], frames[0]) || !bytes.Equal(got[1], frames[1]) {
		t.Errorf("Get returned %v, want %v", got, frames)
	}

	// Miss (different query).
	if miss, _ := rs.Get(ctx, "t1", "db", "select * from orders"); miss != nil {
		t.Errorf("expected miss, got %v", miss)
	}

	// Tenant isolation: same query, different tenant -> miss.
	if miss, _ := rs.Get(ctx, "t2", "db", "select * from users"); miss != nil {
		t.Errorf("expected tenant-isolated miss, got %v", miss)
	}

	// Invalidate by table -> subsequent Get misses.
	if err := rs.InvalidateByTable(ctx, "users"); err != nil {
		t.Fatalf("InvalidateByTable: %v", err)
	}
	if after, _ := rs.Get(ctx, "t1", "db", "select * from users"); after != nil {
		t.Errorf("expected miss after invalidation, got %v", after)
	}
}

func TestExtractTablesFromDDL(t *testing.T) {
	// normalizeQuery upper-cases, so extracted table names come back upper-cased.
	cases := map[string]string{
		"CREATE TABLE foo (id int)":    "FOO",
		"DROP TABLE bar":               "BAR",
		"TRUNCATE TABLE baz":           "BAZ",
		"ALTER TABLE qux ADD c1 int":   "QUX",
		"CREATE TABLE schema1.tbl (x)": "TBL",
	}
	for q, want := range cases {
		tables := ExtractTablesFromQuery(5 /* QueryTypeDDL */, q)
		if len(tables) != 1 || tables[0] != want {
			t.Errorf("ExtractTablesFromQuery(%q) = %v, want [%s]", q, tables, want)
		}
	}
}
