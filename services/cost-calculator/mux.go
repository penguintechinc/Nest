package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/penguintechinc/nest/shared/go_libs/auth"
	"github.com/penguintechinc/nest/shared/go_libs/http/middleware"
	"go.uber.org/zap"
)

func NewMux(calc *Calculator, logger *zap.Logger) http.Handler {
	mux := http.NewServeMux()

	jwksURL := os.Getenv("OIDC_JWKS_URL")
	authMiddleware := middleware.AuthMiddleware(jwksURL, logger)

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	// Protected wrapper
	protected := func(handler http.HandlerFunc) http.Handler {
		return authMiddleware(middleware.TenantFilter(handler))
	}

	// GET /api/v1/billing/{tenantId} - list all months for tenant
	mux.Handle("GET /api/v1/billing/{tenantId}", protected(func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		records := calc.ListRecords(cl.Tenant)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"records": records,
			"count":   len(records),
		})
	}))

	// GET /api/v1/billing/{tenantId}/{month} - get specific month
	mux.Handle("GET /api/v1/billing/{tenantId}/{month}", protected(func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		month := r.PathValue("month")

		record, ok := calc.GetRecord(cl.Tenant, month)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":"not found"}`)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(record)
	}))

	// POST /api/v1/billing/{tenantId}/record - add tokens
	mux.Handle("POST /api/v1/billing/{tenantId}/record", protected(func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())

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

		calc.AddTokens(cl.Tenant, req.ResourceType, req.Tokens)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprint(w, `{"status":"accepted"}`)
	}))

	// GET /api/v1/billing - admin only: all records
	// For simplicity, we check if user has 'admin' role in claims
	mux.Handle("GET /api/v1/billing", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		isAdmin := false
		for _, role := range cl.Roles {
			if role == "admin" || role == "billing-admin" {
				isAdmin = true
				break
			}
		}
		if !isAdmin {
			http.Error(w, "forbidden: admin role required", http.StatusForbidden)
			return
		}

		records := calc.AllRecords()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"records": records,
			"count":   len(records),
		})
	})))

	// GET /api/v1/billing/{tenantId}/summary - aggregate all months
	mux.Handle("GET /api/v1/billing/{tenantId}/summary", protected(func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		records := calc.ListRecords(cl.Tenant)

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
	}))

	// GET /api/v1/billing/{tenantId}/history - daily aggregation history
	mux.Handle("GET /api/v1/billing/{tenantId}/history", protected(func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		history := calc.GetHistory(cl.Tenant)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"history": history,
			"count":   len(history),
		})
	}))

	return mux
}
