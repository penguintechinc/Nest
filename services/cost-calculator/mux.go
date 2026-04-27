package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"go.uber.org/zap"
)

func NewMux(calc *Calculator, logger *zap.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	// GET /api/v1/billing/{tenantId} - list all months for tenant
	mux.HandleFunc("GET /api/v1/billing/{tenantId}", func(w http.ResponseWriter, r *http.Request) {
		tenantID := r.PathValue("tenantId")
		records := calc.ListRecords(tenantID)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"records": records,
			"count":   len(records),
		})
	})

	// GET /api/v1/billing/{tenantId}/{month} - get specific month
	mux.HandleFunc("GET /api/v1/billing/{tenantId}/{month}", func(w http.ResponseWriter, r *http.Request) {
		tenantID := r.PathValue("tenantId")
		month := r.PathValue("month")

		record, ok := calc.GetRecord(tenantID, month)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":"not found"}`)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(record)
	})

	// POST /api/v1/billing/{tenantId}/record - add tokens
	mux.HandleFunc("POST /api/v1/billing/{tenantId}/record", func(w http.ResponseWriter, r *http.Request) {
		tenantID := r.PathValue("tenantId")

		var req struct {
			ResourceType string  `json:"resourceType"`
			Tokens       float64 `json:"tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"invalid request"}`)
			return
		}

		calc.AddTokens(tenantID, req.ResourceType, req.Tokens)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprint(w, `{"status":"accepted"}`)
	})

	// GET /api/v1/billing - admin: all records
	mux.HandleFunc("GET /api/v1/billing", func(w http.ResponseWriter, r *http.Request) {
		records := calc.AllRecords()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"records": records,
			"count":   len(records),
		})
	})

	// GET /api/v1/billing/{tenantId}/summary - aggregate all months
	mux.HandleFunc("GET /api/v1/billing/{tenantId}/summary", func(w http.ResponseWriter, r *http.Request) {
		tenantID := r.PathValue("tenantId")
		records := calc.ListRecords(tenantID)

		totalTokens := 0.0
		totalCostUSD := 0.0
		for _, record := range records {
			totalTokens += record.TotalTokens
			totalCostUSD += record.TotalCostUSD
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"totalTokens":  totalTokens,
			"totalCostUsd": totalCostUSD,
			"months":       len(records),
		})
	})

	return mux
}
