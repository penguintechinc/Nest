package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/penguintechinc/nest/pkg/auth"
	"go.uber.org/zap"
)

func NewMux(calc *Calculator, logger *zap.Logger, authMiddleware *auth.Middleware) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	// GET /api/v1/billing/{tenantId} - list all months for tenant
	mux.Handle("GET /api/v1/billing/{tenantId}", authMiddleware.RequireAuth(authMiddleware.RequireTenant(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			http.Error(w, `{"error": "no claims"}`, http.StatusInternalServerError)
			return
		}

		tenantID := r.PathValue("tenantId")
		// Verify tenant access
		if err := authMiddleware.AssertTenantMatch(claims.Tenant, tenantID); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"error":"tenant mismatch"}`)
			return
		}

		records := calc.ListRecords(tenantID)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"records": records,
			"count":   len(records),
		})
	}))))

	// GET /api/v1/billing/{tenantId}/{month} - get specific month
	mux.Handle("GET /api/v1/billing/{tenantId}/{month}", authMiddleware.RequireAuth(authMiddleware.RequireTenant(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			http.Error(w, `{"error": "no claims"}`, http.StatusInternalServerError)
			return
		}

		tenantID := r.PathValue("tenantId")
		// Verify tenant access
		if err := authMiddleware.AssertTenantMatch(claims.Tenant, tenantID); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"error":"tenant mismatch"}`)
			return
		}

		month := r.PathValue("month")

		record, ok := calc.GetRecord(claims.Tenant, month)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":"not found"}`)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(record)
	}))))

	// POST /api/v1/billing/{tenantId}/record - add tokens
	mux.Handle("POST /api/v1/billing/{tenantId}/record", authMiddleware.RequireAuth(authMiddleware.RequireTenant(authMiddleware.RequireScope("billing:write")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			http.Error(w, `{"error": "no claims"}`, http.StatusInternalServerError)
			return
		}

		tenantID := r.PathValue("tenantId")
		// Verify tenant access
		if err := authMiddleware.AssertTenantMatch(claims.Tenant, tenantID); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"error":"tenant mismatch"}`)
			return
		}

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

		calc.AddTokens(claims.Tenant, req.ResourceType, req.Tokens)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprint(w, `{"status":"accepted"}`)
	})))))

	// GET /api/v1/billing - list all records for token tenant
	mux.Handle("GET /api/v1/billing", authMiddleware.RequireAuth(authMiddleware.RequireTenant(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			http.Error(w, `{"error": "no claims"}`, http.StatusInternalServerError)
			return
		}

		// Return only the token tenant's records
		records := calc.ListRecords(claims.Tenant)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"records": records,
			"count":   len(records),
		})
	}))))

	// GET /api/v1/billing/{tenantId}/summary - aggregate all months
	mux.Handle("GET /api/v1/billing/{tenantId}/summary", authMiddleware.RequireAuth(authMiddleware.RequireTenant(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			http.Error(w, `{"error": "no claims"}`, http.StatusInternalServerError)
			return
		}

		tenantID := r.PathValue("tenantId")
		// Verify tenant access
		if err := authMiddleware.AssertTenantMatch(claims.Tenant, tenantID); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"error":"tenant mismatch"}`)
			return
		}

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
	}))))

	// GET /api/v1/billing/{tenantId}/history - daily aggregation history
	mux.Handle("GET /api/v1/billing/{tenantId}/history", authMiddleware.RequireAuth(authMiddleware.RequireTenant(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			http.Error(w, `{"error": "no claims"}`, http.StatusInternalServerError)
			return
		}

		tenantID := r.PathValue("tenantId")
		// Verify tenant access
		if err := authMiddleware.AssertTenantMatch(claims.Tenant, tenantID); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"error":"tenant mismatch"}`)
			return
		}

		history := calc.GetHistory(tenantID)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"history": history,
			"count":   len(history),
		})
	}))))

	return mux
}
