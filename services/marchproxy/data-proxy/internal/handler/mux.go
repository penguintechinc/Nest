package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"go.uber.org/zap"

	"github.com/penguintechinc/nest/services/marchproxy/data-proxy/internal/claims"
	"github.com/penguintechinc/nest/services/marchproxy/data-proxy/internal/config"
	"github.com/penguintechinc/nest/services/marchproxy/data-proxy/internal/middleware"
)

func NewMux(cfg config.Config, logger *zap.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	// Metrics endpoint
	mux.HandleFunc("GET /metrics/probes", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "# nest_dataresource_health probe metrics\n# Populated by the health prober at runtime\n")
	})

	// Authenticated routes
	authed := http.NewServeMux()
	authed.HandleFunc("POST /api/v1/query", queryHandler(cfg, logger))
	authed.HandleFunc("GET /api/v1/objects/{resource}/{bucket}/{key}", objectGetHandler(cfg, logger))
	authed.HandleFunc("PUT /api/v1/objects/{resource}/{bucket}/{key}", objectPutHandler(cfg, logger))
	authed.HandleFunc("DELETE /api/v1/objects/{resource}/{bucket}/{key}", objectDeleteHandler(cfg, logger))
	authed.HandleFunc("GET /api/v1/volumes/{resource}", volumeInfoHandler(cfg, logger))

	mux.Handle("/api/", middleware.OIDCHTTPMiddleware(cfg, logger, authed))

	return mux
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func queryHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cl, ok := claims.FromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing claims")
			return
		}

		var req struct {
			Resource string `json:"resource"`
			SQL      string `json:"sql"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.Resource == "" {
			writeError(w, http.StatusBadRequest, "resource is required")
			return
		}

		logger.Info("http query",
			zap.String("tenant", cl.Tenant),
			zap.String("resource", req.Resource),
		)

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"tenant":   cl.Tenant,
			"resource": req.Resource,
			"message":  "use native driver at the resource's native endpoint",
		})
	}
}

func objectGetHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cl, ok := claims.FromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing claims")
			return
		}
		resource := r.PathValue("resource")
		_ = r.PathValue("bucket")
		key := r.PathValue("key")

		logger.Info("object GET", zap.String("tenant", cl.Tenant), zap.String("resource", resource), zap.String("key", key))

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"message": "object get endpoint",
		})
	}
}

func objectPutHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cl, _ := claims.FromContext(r.Context())
		resource := r.PathValue("resource")
		logger.Info("object PUT", zap.String("tenant", cl.Tenant), zap.String("resource", resource))
		writeJSON(w, http.StatusOK, map[string]string{"status": "accepted"})
	}
}

func objectDeleteHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cl, _ := claims.FromContext(r.Context())
		resource := r.PathValue("resource")
		logger.Info("object DELETE", zap.String("tenant", cl.Tenant), zap.String("resource", resource))
		w.WriteHeader(http.StatusNoContent)
	}
}

func volumeInfoHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cl, _ := claims.FromContext(r.Context())
		resource := r.PathValue("resource")
		logger.Info("volume INFO", zap.String("tenant", cl.Tenant), zap.String("resource", resource))
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"resource": resource,
			"tenant":   cl.Tenant,
		})
	}
}
