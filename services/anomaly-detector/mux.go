package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"github.com/penguintechinc/nest/pkg/auth"
)

func NewMux(detector *Detector, enterpriseLicense string, logger *slog.Logger, authMiddleware *auth.Middleware) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Wrapper for auth + tenant + WaddleAI license gating
	samplesHandler := authMiddleware.RequireAuth(authMiddleware.RequireTenant(authMiddleware.RequireScope("anomaly:write")(gateWaddleAI(enterpriseLicense, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			http.Error(w, `{"error": "no claims"}`, http.StatusInternalServerError)
			return
		}

		var s MetricSample
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
			return
		}

		// Override tenant with token tenant to prevent baseline-poisoning attacks
		s.Tenant = claims.Tenant

		detector.AddSample(s)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": "added"})
	})))))
	mux.Handle("POST /api/v1/anomaly/samples", samplesHandler)

	currentHandler := authMiddleware.RequireAuth(authMiddleware.RequireTenant(gateWaddleAI(enterpriseLicense, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			http.Error(w, `{"error": "no claims"}`, http.StatusInternalServerError)
			return
		}

		severity := r.URL.Query().Get("severity")
		limit := 50
		if l := r.URL.Query().Get("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil {
				limit = parsed
			}
		}

		// Use token tenant, not query param
		anomalies := detector.GetAnomalies(claims.Tenant, severity, limit)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"anomalies": anomalies,
			"count":     len(anomalies),
		})
	}))))
	mux.Handle("GET /api/v1/anomaly/current", currentHandler)

	statsHandler := authMiddleware.RequireAuth(authMiddleware.RequireTenant(gateWaddleAI(enterpriseLicense, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			http.Error(w, `{"error": "no claims"}`, http.StatusInternalServerError)
			return
		}

		// Use token tenant, not all tenants
		stats := detector.AnomalyStatsForTenant(claims.Tenant)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"stats": stats,
		})
	}))))
	mux.Handle("GET /api/v1/anomaly/stats", statsHandler)

	return mux
}

// gateWaddleAI checks if the enterprise license is valid for WaddleAI features
func gateWaddleAI(enterpriseLicense string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wai := os.Getenv("WADDLEAI_ENABLED")
		lic := enterpriseLicense
		if lic == "" {
			lic = os.Getenv("ENTERPRISE_LICENSE")
		}
		if wai == "" || lic == "" {
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
