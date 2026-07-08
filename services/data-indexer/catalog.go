package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

type ColumnEntry struct {
	Name       string   `json:"name"`
	DataType   string   `json:"dataType,omitempty"`
	Labels     []string `json:"labels,omitempty"`
	Confidence float64  `json:"confidence,omitempty"`
}

type CatalogEntry struct {
	ID              string             `json:"id"`
	Tenant          string             `json:"tenant"`
	ResourceID      string             `json:"resourceId"`
	BackendType     string             `json:"backendType"`
	Namespace       string             `json:"namespace,omitempty"`
	TableName       string             `json:"tableName,omitempty"`
	Columns         []ColumnEntry      `json:"columns,omitempty"`
	DiscoveredAt    time.Time          `json:"discoveredAt"`
	UpdatedAt       time.Time          `json:"updatedAt"`
	Labels          []string           `json:"labels,omitempty"`
	LabelConfidence map[string]float64 `json:"labelConfidence,omitempty"`
}

type Catalog struct {
	mu      sync.RWMutex
	entries map[string]*CatalogEntry
}

func NewCatalog() *Catalog {
	return &Catalog{
		entries: make(map[string]*CatalogEntry),
	}
}

func (c *Catalog) Upsert(entry *CatalogEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.LabelConfidence == nil {
		entry.LabelConfidence = make(map[string]float64)
	}

	key := entry.Tenant + ":" + entry.ResourceID + ":" + entry.TableName
	c.entries[key] = entry
}

func (c *Catalog) Get(id string) (*CatalogEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, e := range c.entries {
		if e.ID == id {
			return e, true
		}
	}
	return nil, false
}

func (c *Catalog) List(tenant, backendType string) []*CatalogEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var result []*CatalogEntry
	for _, e := range c.entries {
		if (tenant == "" || e.Tenant == tenant) &&
			(backendType == "" || e.BackendType == backendType) {
			result = append(result, e)
		}
	}
	return result
}

func (c *Catalog) ApplyLabels(tenant, resourceID, tableName string, labels map[string]float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := tenant + ":" + resourceID + ":" + tableName
	e, ok := c.entries[key]
	if !ok {
		return fmt.Errorf("entry not found: %s", key)
	}

	e.LabelConfidence = labels
	e.Labels = keysOf(labels)
	e.UpdatedAt = time.Now()
	return nil
}

func (c *Catalog) Stats() map[string]int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := make(map[string]int)
	totalLabels := 0
	pendingClassification := 0

	stats["total_entries"] = len(c.entries)
	for _, e := range c.entries {
		totalLabels += len(e.Labels)
		if len(e.Labels) == 0 {
			pendingClassification++
		}
	}
	stats["total_labels"] = totalLabels
	stats["pending_classification"] = pendingClassification

	return stats
}

func keysOf(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
