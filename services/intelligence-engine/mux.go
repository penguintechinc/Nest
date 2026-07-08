package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"

	"github.com/penguintechinc/nest/pkg/auth"
)

func NewMux(classifier *Classifier, logger *slog.Logger, authMiddleware *auth.Middleware) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	classifyHandler := authMiddleware.RequireAuth(authMiddleware.RequireTenant(authMiddleware.RequireScope("intelligence:write")(gateWaddleAI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			http.Error(w, `{"error": "no claims"}`, http.StatusInternalServerError)
			return
		}

		var m WorkloadMetrics
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
			return
		}

		// Override tenant with token tenant
		m.Tenant = claims.Tenant

		rec := classifier.Classify(m)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(rec)
	})))))
	mux.Handle("POST /api/v1/intelligence/classify", classifyHandler)

	recsHandler := authMiddleware.RequireAuth(authMiddleware.RequireTenant(gateWaddleAI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			http.Error(w, `{"error": "no claims"}`, http.StatusInternalServerError)
			return
		}

		// Use token tenant, not query param
		recs := classifier.ListRecommendations(claims.Tenant)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"recommendations": recs,
			"count":           len(recs),
		})
	}))))
	mux.Handle("GET /api/v1/intelligence/recommendations", recsHandler)

	recHandler := authMiddleware.RequireAuth(authMiddleware.RequireTenant(gateWaddleAI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			http.Error(w, `{"error": "no claims"}`, http.StatusInternalServerError)
			return
		}

		resourceID := r.PathValue("resourceId")
		rec, ok := classifier.GetRecommendation(resourceID)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
			return
		}

		// Verify the resource belongs to the token's tenant
		if rec.Tenant != claims.Tenant {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "access denied"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rec)
	}))))
	mux.Handle("GET /api/v1/intelligence/recommendations/{resourceId}", recHandler)

	return mux
}

func gateWaddleAI(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ent := os.Getenv("ENTERPRISE_LICENSE")
		wai := os.Getenv("WADDLEAI_ENABLED")
		if ent == "" || wai == "" {
			w.WriteHeader(http.StatusPaymentRequired)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": "enterprise license required",
				"code":  "nest.enterprise.license_required",
			})
			return
		}
		handler.ServeHTTP(w, r)
	})
}
