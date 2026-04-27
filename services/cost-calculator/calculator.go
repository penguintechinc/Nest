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

// DailyAggregate stores a snapshot of usage for a specific day
type DailyAggregate struct {
	Date    string             `json:"date"` // "2026-04-24" format
	Tenants map[string]*UsageSnapshot `json:"tenants"`
}

// UsageSnapshot stores usage totals for a tenant at a point in time
type UsageSnapshot struct {
	TotalTokens float64            `json:"totalTokens"`
	TotalCostUSD float64           `json:"totalCostUsd"`
	Breakdown   map[string]float64 `json:"breakdown"` // resourceType -> tokenCount
}

// Calculator manages cost records and daily aggregations
type Calculator struct {
	mu             sync.RWMutex
	records        map[string]*CostRecord // key: tenantId+":"+month
	rate           float64                // USD per token
	dailyHistory   []DailyAggregate       // rolling 90-day history
	maxHistoryDays int                    // max days to keep (90)
}

func NewCalculator() *Calculator {
	rate := 0.0001
	if r := os.Getenv("TOKEN_RATE_USD"); r != "" {
		if v, err := strconv.ParseFloat(r, 64); err == nil {
			rate = v
		}
	}
	return &Calculator{
		records:        make(map[string]*CostRecord),
		rate:           rate,
		dailyHistory:   make([]DailyAggregate, 0, 90),
		maxHistoryDays: 90,
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

// RunDailyAggregation runs every 24h and snapshots current usage
func (c *Calculator) RunDailyAggregation(logger *zap.Logger) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()

		today := time.Now().Format("2006-01-02")

		// Create a deep copy of current usage by tenant
		tenantSnapshots := make(map[string]*UsageSnapshot)
		for _, record := range c.records {
			tenantID := record.TenantID
			if _, exists := tenantSnapshots[tenantID]; !exists {
				tenantSnapshots[tenantID] = &UsageSnapshot{
					TotalTokens:  0,
					TotalCostUSD: 0,
					Breakdown:    make(map[string]float64),
				}
			}

			snapshot := tenantSnapshots[tenantID]
			snapshot.TotalTokens += record.TotalTokens
			snapshot.TotalCostUSD += record.TotalCostUSD
			for resource, tokens := range record.Breakdown {
				snapshot.Breakdown[resource] += tokens
			}
		}

		// Create aggregate for today
		aggregate := DailyAggregate{
			Date:    today,
			Tenants: tenantSnapshots,
		}

		// Append to history
		c.dailyHistory = append(c.dailyHistory, aggregate)

		// Prune old entries beyond maxHistoryDays
		if len(c.dailyHistory) > c.maxHistoryDays {
			c.dailyHistory = c.dailyHistory[len(c.dailyHistory)-c.maxHistoryDays:]
		}

		tenantCount := len(tenantSnapshots)
		c.mu.Unlock()

		logger.Info("daily aggregation complete", zap.Int("tenants", tenantCount), zap.String("date", today))
	}
}

// GetHistory returns daily aggregation history for a specific tenant (or all if tenant is empty)
func (c *Calculator) GetHistory(tenant string) []DailyAggregate {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if tenant == "" {
		// Return all aggregates
		result := make([]DailyAggregate, len(c.dailyHistory))
		copy(result, c.dailyHistory)
		return result
	}

	// Filter by tenant
	result := make([]DailyAggregate, 0, len(c.dailyHistory))
	for _, agg := range c.dailyHistory {
		if _, exists := agg.Tenants[tenant]; exists {
			// Create a new aggregate with only this tenant
			filtered := DailyAggregate{
				Date:    agg.Date,
				Tenants: make(map[string]*UsageSnapshot),
			}
			filtered.Tenants[tenant] = agg.Tenants[tenant]
			result = append(result, filtered)
		}
	}
	return result
}
