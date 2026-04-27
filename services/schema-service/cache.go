package main

import (
	"sync"
	"time"
)

type cacheEntry struct {
	schema    *Schema
	expiresAt time.Time
}

// SchemaCache provides thread-safe TTL caching for schema results.
type SchemaCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry // key: resourceID
	ttl     time.Duration
}

// NewSchemaCache creates a new cache with the given TTL.
func NewSchemaCache(ttl time.Duration) *SchemaCache {
	return &SchemaCache{
		entries: make(map[string]*cacheEntry),
		ttl:     ttl,
	}
}

// Get retrieves a schema from the cache if it exists and hasn't expired.
// Returns (schema, true) if found and valid, (nil, false) otherwise.
func (c *SchemaCache) Get(resourceID string) (*Schema, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[resourceID]
	if !exists {
		return nil, false
	}

	// Check if entry has expired
	if time.Now().After(entry.expiresAt) {
		return nil, false
	}

	return entry.schema, true
}

// Set stores a schema in the cache with the configured TTL.
func (c *SchemaCache) Set(resourceID string, schema *Schema) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[resourceID] = &cacheEntry{
		schema:    schema,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// Invalidate removes a schema from the cache.
func (c *SchemaCache) Invalidate(resourceID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, resourceID)
}
