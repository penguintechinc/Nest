package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"go.uber.org/zap"
)

func NewMux(store *ErasureStore, logger *zap.Logger) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	})

	mux.HandleFunc("POST /api/v1/erase", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			SubjectID      string   `json:"subjectId"`
			Tenant         string   `json:"tenant"`
			Async          bool     `json:"async"`
			Backends       []string `json:"backends"`
			IdempotencyKey string   `json:"idempotencyKey"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
			return
		}

		if req.SubjectID == "" || req.Tenant == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "subjectId and tenant required"})
			return
		}

		erasureReq := &ErasureRequest{
			SubjectID:   req.SubjectID,
			Tenant:      req.Tenant,
			Async:       req.Async,
			Backends:    req.Backends,
			Idempotency: req.IdempotencyKey,
		}

		result, err := store.CreateRequest(erasureReq)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		statusCode := http.StatusOK
		if req.Async {
			statusCode = http.StatusAccepted
		}
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(result)
	})

	mux.HandleFunc("GET /api/v1/erase/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/erase/")
		req, ok := store.GetRequest(id)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(req)
	})

	mux.HandleFunc("GET /api/v1/erase", func(w http.ResponseWriter, r *http.Request) {
		tenant := r.URL.Query().Get("tenant")
		if tenant == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "tenant query param required"})
			return
		}

		requests := store.ListRequests(tenant)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(requests)
	})

	return mux
}
