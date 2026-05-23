package pool

import (
	"context"
	"database/sql"
	"sync"
	"time"
)

type ConnectionPool struct {
	driver   string
	dsn      string
	db       *sql.DB
	maxConns int
	mu       sync.RWMutex
}

func NewConnectionPool(driver, dsn string, maxConns int) *ConnectionPool {
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil
	}

	// Performance optimizations for low latency
	db.SetMaxOpenConns(maxConns)
	// Keep more idle connections for faster access
	db.SetMaxIdleConns(int(float64(maxConns) * 0.8)) // 80% instead of 50%
	// Shorter lifetime for fresher connections but not too aggressive
	db.SetConnMaxLifetime(3 * time.Minute)
	// Longer idle time to reduce connection churn
	db.SetConnMaxIdleTime(60 * time.Second)

	pool := &ConnectionPool{
		driver:   driver,
		dsn:      dsn,
		db:       db,
		maxConns: maxConns,
	}

	// Pre-warm the connection pool for better initial performance
	go pool.warmup()

	return pool
}

func (p *ConnectionPool) Get() (*sql.Conn, error) {
	// Use a timeout context to prevent hanging connections
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return p.db.Conn(ctx)
}

func (p *ConnectionPool) GetWithContext(ctx context.Context) (*sql.Conn, error) {
	return p.db.Conn(ctx)
}

func (p *ConnectionPool) Close() error {
	return p.db.Close()
}

func (p *ConnectionPool) Stats() sql.DBStats {
	return p.db.Stats()
}

func (p *ConnectionPool) Ping() error {
	return p.db.Ping()
}

// warmup pre-establishes connections to reduce first-request latency
func (p *ConnectionPool) warmup() {
	// Create some initial connections asynchronously
	warmupCount := int(float64(p.maxConns) * 0.3) // Warm up 30% of max connections
	if warmupCount < 2 {
		warmupCount = 2
	}
	
	for i := 0; i < warmupCount; i++ {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			
			if conn, err := p.db.Conn(ctx); err == nil {
				// Just ping and close to establish connection in pool
				if err := conn.PingContext(ctx); err == nil {
					conn.Close() // Returns to pool
				}
			}
		}()
	}
}