package main

import (
	"encoding/json"
	"net/http"
	"time"

	"go.uber.org/zap"
)

type ScanRequest struct {
	ResourceID  string `json:"resourceId"`
	BackendType string `json:"backendType"`
	Tenant      string `json:"tenant"`
	Tables      []struct {
		Name    string `json:"name"`
		Columns []struct {
			Name     string `json:"name"`
			DataType string `json:"dataType"`
		} `json:"columns"`
	} `json:"tables"`
}

type ScanResponse struct {
	Status        string `json:"status"`
	EntriesQueued int    `json:"entriesQueued"`
}

type ClassifyRequest struct {
	ResourceID string `json:"resourceId"`
	TableName  string `json:"tableName"`
}

type ListResponse struct {
	Entries []*CatalogEntry `json:"entries"`
	Count   int             `json:"count"`
	Stats   map[string]int  `json:"stats"`
}

type LabelsResponse struct {
	Targets []*CatalogEntry `json:"targets"`
}

func NewMux(catalog *Catalog, pipeline *Pipeline, logger *zap.Logger) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /api/v1/indexer/scan", func(w http.ResponseWriter, r *http.Request) {
		var req ScanRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		queued := 0
		for _, tbl := range req.Tables {
			entry := &CatalogEntry{
				Tenant:       req.Tenant,
				ResourceID:   req.ResourceID,
				BackendType:  req.BackendType,
				TableName:    tbl.Name,
				DiscoveredAt: time.Now(),
				UpdatedAt:    time.Now(),
			}
			for _, col := range tbl.Columns {
				entry.Columns = append(entry.Columns, ColumnEntry{
					Name:     col.Name,
					DataType: col.DataType,
				})
			}
			catalog.Upsert(entry)
			queued++

			go func(resID, tblName string) {
				if err := pipeline.ClassifyEntry(resID, tblName); err != nil {
					logger.Error("async classify error", zap.String("resource", resID), zap.String("table", tblName), zap.Error(err))
				}
			}(req.ResourceID, tbl.Name)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(ScanResponse{Status: "scanning", EntriesQueued: queued})
	})

	mux.HandleFunc("GET /api/v1/indexer/catalog", func(w http.ResponseWriter, r *http.Request) {
		tenant := r.URL.Query().Get("tenant")
		backend := r.URL.Query().Get("backend")

		entries := catalog.List(tenant, backend)
		resp := ListResponse{
			Entries: entries,
			Count:   len(entries),
			Stats:   catalog.Stats(),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("GET /api/v1/indexer/catalog/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		entry, ok := catalog.Get(id)
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entry)
	})

	mux.HandleFunc("GET /api/v1/indexer/labels", func(w http.ResponseWriter, r *http.Request) {
		resource := r.URL.Query().Get("resource")
		table := r.URL.Query().Get("table")

		catalog.mu.RLock()
		var filtered []*CatalogEntry
		for _, e := range catalog.entries {
			if (resource == "" || e.ResourceID == resource) &&
				(table == "" || e.TableName == table) {
				filtered = append(filtered, e)
			}
		}
		catalog.mu.RUnlock()

		resp := LabelsResponse{Targets: filtered}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("POST /api/v1/indexer/classify", func(w http.ResponseWriter, r *http.Request) {
		var req ClassifyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		if err := pipeline.ClassifyEntry(req.ResourceID, req.TableName); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		key := req.ResourceID + ":" + req.TableName
		catalog.mu.RLock()
		entry, ok := catalog.entries[key]
		catalog.mu.RUnlock()

		if !ok {
			http.Error(w, "entry not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entry)
	})

	mux.HandleFunc("GET /api/v1/indexer/stats", func(w http.ResponseWriter, r *http.Request) {
		stats := catalog.Stats()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	})

	mux.HandleFunc("GET /api/v1/indexer/pii-targets", func(w http.ResponseWriter, r *http.Request) {
		catalog.mu.RLock()
		var targets []*CatalogEntry
		for _, e := range catalog.entries {
			for _, label := range e.Labels {
				if label == "PII" {
					targets = append(targets, e)
					break
				}
			}
		}
		catalog.mu.RUnlock()

		resp := LabelsResponse{Targets: targets}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	return mux
}
