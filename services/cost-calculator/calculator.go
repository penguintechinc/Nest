package main

import (
	"os"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"
)

// CostRecord stores per-tenant per-month cost
type CostRecord struct {
	TenantID    string             `json:"tenantId"`
	Month       string             `json:"month"` // "2025-01" format
	TotalTokens float64            `json:"totalTokens"`
	TotalCostUSD float64           `json:"totalCostUsd"`
	Breakdown   map[string]float64 `json:"breakdown"` // resourceType -> tokenCount
	UpdatedAt   time.Time          `json:"updatedAt"`
}

// Calculator manages cost records
type Calculator struct {
	mu      sync.RWMutex
	records map[string]*CostRecord // key: tenantId+":"+month
	rate    float64                // USD per token
}

func NewCalculator() *Calculator {
	rate := 0.0001
	if r := os.Getenv("TOKEN_RATE_USD"); r != "" {
		if v, err := strconv.ParseFloat(r, 64); err == nil {
			rate = v
		}
	}
	return &Calculator{
		records: make(map[string]*CostRecord),
		rate:    rate,
	}
}

// AddTokens upserts record for current month
func (c *Calculator) AddTokens(tenantID, resourceType string, tokens float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	month := now.Format("2006-01")
	key := tenantID + ":" + month

	if record, ok := c.records[key]; ok {
		record.TotalTokens += tokens
		record.TotalCostUSD = record.TotalTokens * c.rate
		if record.Breakdown == nil {
			record.Breakdown = make(map[string]float64)
		}
		record.Breakdown[resourceType] += tokens
		record.UpdatedAt = now
	} else {
		breakdown := make(map[string]float64)
		breakdown[resourceType] = tokens
		c.records[key] = &CostRecord{
			TenantID:    tenantID,
			Month:       month,
			TotalTokens: tokens,
			TotalCostUSD: tokens * c.rate,
			Breakdown:   breakdown,
			UpdatedAt:   now,
		}
	}
}

// GetRecord retrieves record for tenant and month
func (c *Calculator) GetRecord(tenantID, month string) (*CostRecord, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := tenantID + ":" + month
	record, ok := c.records[key]
	return record, ok
}

// ListRecords returns all records for a tenant
func (c *Calculator) ListRecords(tenantID string) []*CostRecord {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var result []*CostRecord
	for _, record := range c.records {
		if record.TenantID == tenantID {
			result = append(result, record)
		}
	}
	return result
}

// AllRecords returns all records
func (c *Calculator) AllRecords() []*CostRecord {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var result []*CostRecord
	for _, record := range c.records {
		result = append(result, record)
	}
	return result
}

// RunDailyAggregation runs every 24h (stub implementation)
func (c *Calculator) RunDailyAggregation(logger *zap.Logger) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		logger.Info("running daily aggregation")
		// TODO: query meter events from database and aggregate
	}
}
