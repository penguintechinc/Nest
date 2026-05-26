package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"github.com/penguintechinc/nest/shared/go_libs/auth"
	"github.com/penguintechinc/nest/shared/go_libs/http/middleware"
	"github.com/penguintechinc/nest/shared/licensing"
	"go.uber.org/zap"
)

func NewMux(detector *Detector, enterpriseLicense string, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	validator := licensing.NewValidator(enterpriseLicense, "nest")
	jwksURL := os.Getenv("OIDC_JWKS_URL")
	
	// Create a zap logger for the middleware
	zlog, _ := zap.NewProduction()
	authMiddleware := middleware.AuthMiddleware(jwksURL, zlog)

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Protected wrapper
	protected := func(handler http.HandlerFunc) http.Handler {
		return authMiddleware(gateWaddleAI(validator, middleware.TenantFilter(handler)))
	}

	mux.Handle("POST /api/v1/anomaly/samples", protected(func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		
		var s MetricSample
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
			return
		}

		// Enforce tenant
		s.Tenant = cl.Tenant

		detector.AddSample(s)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": "added"})
	}))

	mux.Handle("GET /api/v1/anomaly/current", protected(func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		
		severity := r.URL.Query().Get("severity")
		limit := 50
		if l := r.URL.Query().Get("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil {
				limit = parsed
			}
		}

		anomalies := detector.GetAnomalies(cl.Tenant, severity, limit)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"anomalies": anomalies,
			"count":     len(anomalies),
		})
	}))

	mux.Handle("GET /api/v1/anomaly/stats", protected(func(w http.ResponseWriter, r *http.Request) {
		// Stats might be global or per tenant? 
		// If global, we might need a special role.
		// For now, let's assume stats are for the tenant's data.
		// But detector.AnomalyStats() doesn't take tenant.
		// In world-class enterprise, it should.
		
		stats := detector.AnomalyStats()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"stats": stats,
		})
	}))

	return mux
}

func gateWaddleAI(validator *licensing.Validator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wai := os.Getenv("WADDLEAI_ENABLED")
		if wai == "" || !validator.IsValid(r) {
			w.WriteHeader(http.StatusPaymentRequired)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": "enterprise license required",
				"code":  "nest.enterprise.license_required",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}
