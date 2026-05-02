package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"go.uber.org/zap"

	"github.com/penguintechinc/nest/services/gateway/internal/auth"
	"github.com/penguintechinc/nest/services/gateway/internal/claims"
	"github.com/penguintechinc/nest/services/gateway/internal/config"
	"github.com/penguintechinc/nest/services/gateway/internal/middleware"
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
	authed.HandleFunc("GET /api/v1/tenants/{tid}/resources", storageListHandler(cfg, logger))
	authed.HandleFunc("GET /api/v1/tenants/{tid}/resources/{name}", storageGetHandler(cfg, logger))
	authed.HandleFunc("POST /api/v1/tenants/{tid}/resources", storageCreateHandler(cfg, logger))
	authed.HandleFunc("DELETE /api/v1/tenants/{tid}/resources/{name}", storageDeleteHandler(cfg, logger))
	authed.HandleFunc("GET /api/v1/tenants/{tid}/databases", dbListHandler(cfg, logger))
	authed.HandleFunc("GET /api/v1/tenants/{tid}/databases/{name}", dbGetHandler(cfg, logger))
	authed.HandleFunc("POST /api/v1/tenants/{tid}/databases", dbCreateHandler(cfg, logger))
	authed.HandleFunc("DELETE /api/v1/tenants/{tid}/databases/{name}", dbDeleteHandler(cfg, logger))
	authed.HandleFunc("GET /api/v1/tenants/{tid}/engines", extListHandler(cfg, logger))
	authed.HandleFunc("GET /api/v1/tenants/{tid}/engines/{name}", extGetHandler(cfg, logger))
	authed.HandleFunc("POST /api/v1/tenants/{tid}/engines", extCreateHandler(cfg, logger))
	authed.HandleFunc("DELETE /api/v1/tenants/{tid}/engines/{name}", extDeleteHandler(cfg, logger))
	authed.HandleFunc("GET /api/v1/tenants/{tid}/warehouses", warehouseListHandler(cfg, logger))
	authed.HandleFunc("GET /api/v1/tenants/{tid}/warehouses/{name}", warehouseGetHandler(cfg, logger))
	authed.HandleFunc("POST /api/v1/tenants/{tid}/warehouses", warehouseCreateHandler(cfg, logger))
	authed.HandleFunc("DELETE /api/v1/tenants/{tid}/warehouses/{name}", warehouseDeleteHandler(cfg, logger))
	authed.HandleFunc("POST /api/v1/tenants/{tid}/resources/{name}/import", importHandler(cfg, logger))
	authed.HandleFunc("POST /api/v1/tenants/{tid}/resources/{name}/export", exportHandler(cfg, logger))
	authed.HandleFunc("GET /api/v1/tenants/{tid}/dataresources", dataresourceListHandler(cfg, logger))
	authed.HandleFunc("GET /api/v1/tenants/{tid}/dataresources/{name}", dataresourceGetHandler(cfg, logger))
	authed.HandleFunc("POST /api/v1/tenants/{tid}/dataresources", dataresourceCreateHandler(cfg, logger))
	authed.HandleFunc("PATCH /api/v1/tenants/{tid}/dataresources/{name}", dataresourcePatchHandler(cfg, logger))
	authed.HandleFunc("DELETE /api/v1/tenants/{tid}/dataresources/{name}", dataresourceDeleteHandler(cfg, logger))
	authed.HandleFunc("GET /api/v1/evidence/compliance/{bundle}", evidenceHandler(cfg, logger))
	authed.HandleFunc("GET /api/v1/tenants/{tid}/indexer/catalog", indexerCatalogHandler(cfg, logger))
	authed.HandleFunc("POST /api/v1/tenants/{tid}/indexer/scan", indexerScanHandler(cfg, logger))
	authed.HandleFunc("GET /api/v1/tenants/{tid}/indexer/pii-targets", indexerPIITargetsHandler(cfg, logger))
	authed.HandleFunc("POST /api/v1/tenants/{tid}/policy/evaluate", policyEvaluateHandler(cfg, logger))
	authed.HandleFunc("GET /api/v1/tenants/{tid}/metering", meteringHandler(cfg, logger))
	authed.HandleFunc("GET /api/v1/tenants/{tid}/billing", billingHandler(cfg, logger))
	authed.HandleFunc("GET /api/v1/tenants/{tid}/intelligence/recommend", intelligenceRecommendHandler(cfg, logger))
	authed.HandleFunc("GET /api/v1/tenants/{tid}/predictive-drive/risk", predictiveDriveHandler(cfg, logger))
	authed.HandleFunc("GET /api/v1/tenants/{tid}/anomaly/current", anomalyDetectHandler(cfg, logger))

	mux.Handle("/api/", middleware.OIDCHTTPMiddleware(cfg, logger, authed))

	// SAML endpoints (no JWT auth required)
	mux.HandleFunc("POST /saml/acs", auth.SAMLACSHandler)
	mux.HandleFunc("GET /saml/metadata", auth.SAMLMetadataHandler)

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
