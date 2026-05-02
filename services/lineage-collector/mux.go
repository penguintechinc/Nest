package main

import (
	"encoding/json"
	"net/http"
	"strconv"

	"go.uber.org/zap"
)

func NewMux(store *LineageStore, logger *zap.Logger) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	handleLineageIngest := func(w http.ResponseWriter, r *http.Request) {
		var e LineageEvent
		if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		tenant := r.Header.Get("X-Nest-Tenant")
		e.Tenant = tenant

		store.Append(&e)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(e)
	}

	mux.HandleFunc("POST /api/v1/lineage/events", handleLineageIngest)
	mux.HandleFunc("POST /api/v1/lineage", handleLineageIngest)

	mux.HandleFunc("GET /api/v1/lineage/events", func(w http.ResponseWriter, r *http.Request) {
		tenant := r.URL.Query().Get("tenant")
		jobName := r.URL.Query().Get("job")
		runID := r.URL.Query().Get("run_id")

		limit := 0
		if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
			limit, _ = strconv.Atoi(limitStr)
		}

		events := store.Query(tenant, jobName, runID, limit)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"events": events,
			"count":  len(events),
		})
	})

	mux.HandleFunc("GET /api/v1/lineage/datasets/{namespace}/{name}", func(w http.ResponseWriter, r *http.Request) {
		namespace := r.PathValue("namespace")
		name := r.PathValue("name")

		events := store.LineageForDataset(namespace, name)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(events)
	})

	return mux
}
