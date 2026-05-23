package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
)

func NewMux(predictor *DrivePredictor, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /api/v1/predictive-drive/assess", gateWaddleAI(func(w http.ResponseWriter, r *http.Request) {
		var m DriveMetrics
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
			return
		}

		assessment := predictor.Assess(m)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(assessment)
	}))

	mux.HandleFunc("GET /api/v1/predictive-drive/assessments", gateWaddleAI(func(w http.ResponseWriter, r *http.Request) {
		node := r.URL.Query().Get("node")
		assessments := predictor.ListAssessments(node)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"assessments": assessments,
			"count":       len(assessments),
		})
	}))

	mux.HandleFunc("GET /api/v1/predictive-drive/risk", gateWaddleAI(func(w http.ResponseWriter, r *http.Request) {
		risk := predictor.HighRisk()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"high_risk": risk,
			"count":     len(risk),
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
