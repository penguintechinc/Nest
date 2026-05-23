package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"
)

func NewMux(detector *Detector, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /api/v1/anomaly/samples", gateWaddleAI(func(w http.ResponseWriter, r *http.Request) {
		var s MetricSample
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
			return
		}

		detector.AddSample(s)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": "added"})
	}))

	mux.HandleFunc("GET /api/v1/anomaly/current", gateWaddleAI(func(w http.ResponseWriter, r *http.Request) {
		tenant := r.URL.Query().Get("tenant")
		severity := r.URL.Query().Get("severity")
		limit := 50
		if l := r.URL.Query().Get("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil {
				limit = parsed
			}
		}

		anomalies := detector.GetAnomalies(tenant, severity, limit)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"anomalies": anomalies,
			"count":     len(anomalies),
		})
	}))

	mux.HandleFunc("GET /api/v1/anomaly/stats", gateWaddleAI(func(w http.ResponseWriter, r *http.Request) {
		stats := detector.AnomalyStats()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"stats": stats,
		})
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
