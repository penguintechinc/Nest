package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"
)

func NewMux(forecaster *Forecaster, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /api/v1/predictor/samples", gateWaddleAI(func(w http.ResponseWriter, r *http.Request) {
		var s UtilizationSample
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
			return
		}

		forecaster.AddSample(s)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": "added"})
	}))

	mux.HandleFunc("POST /api/v1/predictor/forecast", gateWaddleAI(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ResourceID string `json:"resourceId"`
			HoursAhead int    `json:"hoursAhead"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
			return
		}

		forecast, err := forecaster.Forecast(req.ResourceID, req.HoursAhead)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "forecast failed"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(forecast)
	}))

	mux.HandleFunc("GET /api/v1/predictor/scale-needed", gateWaddleAI(func(w http.ResponseWriter, r *http.Request) {
		hoursAhead := 24
		if ha := r.URL.Query().Get("hoursAhead"); ha != "" {
			if h, err := strconv.Atoi(ha); err == nil {
				hoursAhead = h
			}
		}

		resources := forecaster.ListResources()
		var scaleNeeded []*ScaleForecast

		for _, rid := range resources {
			forecast, _ := forecaster.Forecast(rid, hoursAhead)
			if forecast.ScaleRecommended {
				scaleNeeded = append(scaleNeeded, forecast)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"scale_needed": scaleNeeded,
			"count":        len(scaleNeeded),
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
