package main

import (
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
)

type Pipeline struct {
	catalog *Catalog
	logger  *zap.Logger
}

func NewPipeline(catalog *Catalog, logger *zap.Logger) *Pipeline {
	return &Pipeline{catalog: catalog, logger: logger}
}

func (p *Pipeline) ClassifyEntry(tenant, resourceID, tableName string) error {
	p.catalog.mu.RLock()
	key := tenant + ":" + resourceID + ":" + tableName
	entry, ok := p.catalog.entries[key]
	if !ok {
		p.catalog.mu.RUnlock()
		return fmt.Errorf("entry not found: %s", key)
	}
	cols := make([]ColumnEntry, len(entry.Columns))
	copy(cols, entry.Columns)
	p.catalog.mu.RUnlock()

	labelConfidence := make(map[string]float64)
	for i, col := range cols {
		results := ClassifyColumn(col.Name, col.DataType)
		colLabels := []string{}
		for _, r := range results {
			colLabels = append(colLabels, r.Label)
			if r.Confidence > labelConfidence[r.Label] {
				labelConfidence[r.Label] = r.Confidence
			}
		}
		cols[i].Labels = colLabels
		if len(results) > 0 {
			cols[i].Confidence = results[0].Confidence
		}
	}

	p.catalog.mu.Lock()
	defer p.catalog.mu.Unlock()
	if e, ok := p.catalog.entries[key]; ok {
		e.Columns = cols
		e.Labels = keysOf(labelConfidence)
		e.LabelConfidence = labelConfidence
		e.UpdatedAt = time.Now()
	}
	return nil
}

func (p *Pipeline) performScan(logger *zap.Logger) {
	p.catalog.mu.RLock()
	keys := make([]string, 0, len(p.catalog.entries))
	for k := range p.catalog.entries {
		keys = append(keys, k)
	}
	p.catalog.mu.RUnlock()

	for _, key := range keys {
		// Keys are tenant:resourceID:tableName
		parts := strings.SplitN(key, ":", 3)
		if len(parts) == 3 {
			if err := p.ClassifyEntry(parts[0], parts[1], parts[2]); err != nil {
				logger.Error("classification error", zap.String("key", key), zap.Error(err))
			}
		}
	}
}

func (p *Pipeline) RunDailyScans(logger *zap.Logger) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		p.performScan(logger)
	}
}
