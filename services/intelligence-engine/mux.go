package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"

	"github.com/penguintechinc/nest/shared/go_libs/auth"
	"github.com/penguintechinc/nest/shared/go_libs/http/middleware"
	"github.com/penguintechinc/nest/shared/licensing"
	"go.uber.org/zap"
)

func NewMux(classifier *Classifier, enterpriseLicense string, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	validator := licensing.NewValidator(enterpriseLicense, "nest")
	jwksURL := os.Getenv("OIDC_JWKS_URL")
	
	zlog, _ := zap.NewProduction()
	authMiddleware := middleware.AuthMiddleware(jwksURL, zlog)

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	protected := func(handler http.HandlerFunc) http.Handler {
		return authMiddleware(gateWaddleAI(validator, middleware.TenantFilter(handler)))
	}

	mux.Handle("POST /api/v1/intelligence/classify", protected(func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		var m WorkloadMetrics
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
			return
		}

		m.Tenant = cl.Tenant

		rec := classifier.Classify(m)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(rec)
	}))

	mux.Handle("GET /api/v1/intelligence/recommendations", protected(func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		recs := classifier.ListRecommendations(cl.Tenant)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"recommendations": recs,
			"count":           len(recs),
		})
	}))

	mux.Handle("GET /api/v1/intelligence/recommendations/{resourceId}", protected(func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		resourceID := r.PathValue("resourceId")
		rec, ok := classifier.GetRecommendation(resourceID)
		if !ok || (rec.Tenant != "" && rec.Tenant != cl.Tenant) {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found or unauthorized"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rec)
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
