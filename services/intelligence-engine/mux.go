package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
)

func NewMux(classifier *Classifier, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /api/v1/intelligence/classify", gateWaddleAI(func(w http.ResponseWriter, r *http.Request) {
		var m WorkloadMetrics
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
			return
		}

		rec := classifier.Classify(m)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(rec)
	}))

	mux.HandleFunc("GET /api/v1/intelligence/recommendations", gateWaddleAI(func(w http.ResponseWriter, r *http.Request) {
		tenant := r.URL.Query().Get("tenant")
		recs := classifier.ListRecommendations(tenant)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"recommendations": recs,
			"count":           len(recs),
		})
	}))

	mux.HandleFunc("GET /api/v1/intelligence/recommendations/{resourceId}", gateWaddleAI(func(w http.ResponseWriter, r *http.Request) {
		resourceID := r.PathValue("resourceId")
		rec, ok := classifier.GetRecommendation(resourceID)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rec)
	}))

	return mux
}

func gateWaddleAI(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		handler(w, r)
	}
}
