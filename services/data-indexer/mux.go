package main

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/penguintechinc/nest/shared/go_libs/auth"
	"github.com/penguintechinc/nest/shared/go_libs/http/middleware"
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

func NewMux(catalog *Catalog, pipeline *Pipeline, logger *zap.Logger) http.Handler {
	mux := http.NewServeMux()

	jwksURL := os.Getenv("OIDC_JWKS_URL")
	authMiddleware := middleware.AuthMiddleware(jwksURL, logger)

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Protected wrapper
	protected := func(handler http.HandlerFunc) http.Handler {
		return authMiddleware(middleware.TenantFilter(handler))
	}

	mux.Handle("POST /api/v1/indexer/scan", protected(func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		var req ScanRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		// Enforce tenant from claims
		req.Tenant = cl.Tenant

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
	}))

	mux.Handle("GET /api/v1/indexer/catalog", protected(func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		backend := r.URL.Query().Get("backend")

		entries := catalog.List(cl.Tenant, backend)
		resp := ListResponse{
			Entries: entries,
			Count:   len(entries),
			Stats:   catalog.Stats(),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))

	mux.Handle("GET /api/v1/indexer/catalog/{id}", protected(func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		id := r.PathValue("id")
		entry, ok := catalog.Get(id)
		if !ok || (entry.Tenant != "" && entry.Tenant != cl.Tenant) {
			http.Error(w, "not found or unauthorized", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entry)
	}))

	mux.Handle("GET /api/v1/indexer/labels", protected(func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		resource := r.URL.Query().Get("resource")
		table := r.URL.Query().Get("table")

		catalog.mu.RLock()
		var filtered []*CatalogEntry
		for _, e := range catalog.entries {
			if e.Tenant == cl.Tenant &&
				(resource == "" || e.ResourceID == resource) &&
				(table == "" || e.TableName == table) {
				filtered = append(filtered, e)
			}
		}
		catalog.mu.RUnlock()

		resp := LabelsResponse{Targets: filtered}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))

	mux.Handle("POST /api/v1/indexer/classify", protected(func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		var req ClassifyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		// Check if entry exists and belongs to tenant
		key := req.ResourceID + ":" + req.TableName
		catalog.mu.RLock()
		existing, ok := catalog.entries[key]
		catalog.mu.RUnlock()

		if !ok || existing.Tenant != cl.Tenant {
			http.Error(w, "entry not found or unauthorized", http.StatusNotFound)
			return
		}

		if err := pipeline.ClassifyEntry(req.ResourceID, req.TableName); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		catalog.mu.RLock()
		entry := catalog.entries[key]
		catalog.mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entry)
	}))

	mux.Handle("GET /api/v1/indexer/stats", protected(func(w http.ResponseWriter, r *http.Request) {
		// Stats currently global in Catalog, but should be per tenant.
		// For now we'll just return global stats but it's a known gap.
		stats := catalog.Stats()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	}))

	mux.Handle("GET /api/v1/indexer/pii-targets", protected(func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		catalog.mu.RLock()
		var targets []*CatalogEntry
		for _, e := range catalog.entries {
			if e.Tenant == cl.Tenant {
				for _, label := range e.Labels {
					if label == "PII" {
						targets = append(targets, e)
						break
					}
				}
			}
		}
		catalog.mu.RUnlock()

		resp := LabelsResponse{Targets: targets}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))

	return mux
}
