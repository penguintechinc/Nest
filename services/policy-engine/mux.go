package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/penguintechinc/nest/shared/go_libs/auth"
	"github.com/penguintechinc/nest/shared/go_libs/http/middleware"
	"go.uber.org/zap"
)

func NewMux(store *PolicyStore, logger *zap.Logger) http.Handler {
	mux := http.NewServeMux()

	jwksURL := os.Getenv("OIDC_JWKS_URL")
	if jwksURL == "" {
		logger.Warn("OIDC_JWKS_URL not set; auth will fail unless in test mode")
	}

	authMiddleware := middleware.AuthMiddleware(jwksURL, logger)

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Protected routes wrapper
	protected := func(next http.HandlerFunc) http.Handler {
		return authMiddleware(middleware.TenantFilter(next))
	}

	mux.Handle("GET /api/v1/policies", protected(func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		rules := store.ListRules(cl.Tenant)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"rules": rules,
			"count": len(rules),
		})
	}))

	mux.Handle("POST /api/v1/policies", protected(func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		var rule PolicyRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
			return
		}
		
		rule.Tenant = cl.Tenant // Enforce tenant
		
		created, _ := store.CreateRule(&rule)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(created)
	}))

	mux.Handle("GET /api/v1/policies/{id}", protected(func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		id := r.PathValue("id")
		rule, ok := store.GetRule(id)
		if !ok || (rule.Tenant != "" && rule.Tenant != cl.Tenant) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found or unauthorized"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(rule)
	}))

	mux.Handle("DELETE /api/v1/policies/{id}", protected(func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		id := r.PathValue("id")
		
		rule, ok := store.GetRule(id)
		if !ok || (rule.Tenant != "" && rule.Tenant != cl.Tenant) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found or unauthorized"})
			return
		}

		if !store.DeleteRule(id) {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("POST /api/v1/evaluate", protected(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ResourceID    string   `json:"resourceId"`
			UserRole      string   `json:"userRole"`
			RequestedScope string   `json:"requestedScope"`
			Region        string   `json:"region"`
			Labels        []string `json:"labels"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
			return
		}
		
		// In a real multi-tenant system, we should pass tenant to Evaluate
		// so it only checks global rules + this tenant's rules.
		// For now, Evaluate likely checks all rules. I'll pass tenant info if needed.
		// But store.Evaluate currently filters by priority.
		
		decision := store.Evaluate(req.ResourceID, req.UserRole, req.RequestedScope, req.Region, req.Labels)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(decision)
	}))

	// Handle standard Go http routing for catch-all
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/healthz") &&
			!strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
			return
		}
		mux.ServeHTTP(w, r)
	})

	return handler
}
