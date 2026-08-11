package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"go.uber.org/zap"
)

// authMiddleware validates Bearer JWT tokens and enforces tenant claim.
// P2: real JWT validation via penguin-libs auth middleware.
// For now: minimal implementation that checks for Bearer token presence.
func authMiddleware(handler func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"code": "nest.auth.missing_token", "message": "missing Authorization header"})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"code": "nest.auth.invalid_token", "message": "invalid Authorization header format"})
			return
		}

		token := parts[1]
		if token == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"code": "nest.auth.empty_token", "message": "empty token"})
			return
		}

		// P2: real JWT decode and validation via penguin-libs auth middleware.
		// For now, accept any non-empty Bearer token and extract tenant from request body.
		handler(w, r)
	}
}

func NewMux(store *ErasureStore, logger *zap.Logger) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	})

	mux.HandleFunc("POST /api/v1/erase", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
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
	}))

	mux.HandleFunc("GET /api/v1/erase/{id}", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
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
	}))

	mux.HandleFunc("GET /api/v1/erase", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
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
	}))

	return mux
}
