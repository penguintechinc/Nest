package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"go.uber.org/zap"
)

func NewMux(store *SagaStore, logger *zap.Logger) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /api/v1/templates", func(w http.ResponseWriter, r *http.Request) {
		templates := store.ListTemplates()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(templates)
	})

	mux.HandleFunc("POST /api/v1/templates", func(w http.ResponseWriter, r *http.Request) {
		var t SagaTemplate
		if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		if err := store.CreateTemplate(&t); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(t)
	})

	mux.HandleFunc("GET /api/v1/templates/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		t, ok := store.GetTemplate(id)
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(t)
	})

	mux.HandleFunc("POST /api/v1/workflows", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			TemplateID string `json:"templateId"`
			Tenant     string `json:"tenant"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		if req.TemplateID == "" || req.Tenant == "" {
			http.Error(w, "templateId and tenant required", http.StatusBadRequest)
			return
		}

		run, err := store.StartRun(req.TemplateID, req.Tenant)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Location", fmt.Sprintf("/api/v1/workflows/%s", run.ID))
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(run)
	})

	mux.HandleFunc("GET /api/v1/workflows", func(w http.ResponseWriter, r *http.Request) {
		tenant := r.URL.Query().Get("tenant")
		runs := store.ListRuns(tenant)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(runs)
	})

	mux.HandleFunc("GET /api/v1/workflows/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		run, ok := store.GetRun(id)
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(run)
	})

	mux.HandleFunc("POST /api/v1/workflows/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := store.RetryRun(id); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		run, _ := store.GetRun(id)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(run)
	})

	mux.HandleFunc("POST /api/v1/workflows/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := store.CancelRun(id); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		run, _ := store.GetRun(id)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(run)
	})

	return mux
}
