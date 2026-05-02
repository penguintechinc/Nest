package main

import (
	"context"
	"encoding/json"
	"net/http"

	"go.uber.org/zap"
)

// IntrospectorInterface defines the interface for schema introspection.
type IntrospectorInterface interface {
	Introspect(ctx context.Context, resourceID, resourceType, endpoint string) (*Schema, error)
}

// NewMux creates the HTTP router with all schema service endpoints.
func NewMux(cache *SchemaCache, introspector IntrospectorInterface, logger *zap.Logger) *http.ServeMux {
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// GET /api/v1/schemas/{resourceId} - Get schema from cache or introspect
	mux.HandleFunc("GET /api/v1/schemas/{resourceId}", func(w http.ResponseWriter, r *http.Request) {
		resourceID := r.PathValue("resourceId")
		refresh := r.URL.Query().Get("refresh") == "true"

		logger.Debug("schema request", zap.String("resourceID", resourceID), zap.Bool("refresh", refresh))

		// Check cache if not forcing refresh
		if !refresh {
			if cached, found := cache.Get(resourceID); found {
				logger.Debug("cache hit", zap.String("resourceID", resourceID))
				writeJSON(w, http.StatusOK, cached)
				return
			}
		}

		// Cache miss or refresh requested - introspect
		resourceType := r.URL.Query().Get("type")
		endpoint := r.URL.Query().Get("endpoint")

		if resourceType == "" {
			writeError(w, http.StatusBadRequest, "missing required query parameter: type")
			return
		}

		schema, err := introspector.Introspect(r.Context(), resourceID, resourceType, endpoint)
		if err != nil {
			logger.Error("introspection failed", zap.String("resourceID", resourceID), zap.Error(err))
			writeError(w, http.StatusInternalServerError, "introspection failed")
			return
		}

		// Cache the result
		cache.Set(resourceID, schema)

		logger.Debug("introspection complete", zap.String("resourceID", resourceID))
		writeJSON(w, http.StatusOK, schema)
	})

	// DELETE /api/v1/schemas/{resourceId} - Invalidate cache entry
	mux.HandleFunc("DELETE /api/v1/schemas/{resourceId}", func(w http.ResponseWriter, r *http.Request) {
		resourceID := r.PathValue("resourceId")

		logger.Debug("invalidating cache", zap.String("resourceID", resourceID))
		cache.Invalidate(resourceID)

		writeJSON(w, http.StatusOK, map[string]string{"status": "invalidated"})
	})

	return mux
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
