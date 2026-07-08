package main

import (
	"encoding/json"
	"net/http"

	"github.com/penguintechinc/nest/pkg/auth"
	"go.uber.org/zap"
)

func NewMux(store *PolicyStore, logger *zap.Logger, authMiddleware *auth.Middleware) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /healthz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))

	mux.Handle("GET /api/v1/policies", authMiddleware.RequireAuth(authMiddleware.RequireTenant(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Derive tenant from verified token, not request
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			http.Error(w, `{"error": "no claims"}`, http.StatusInternalServerError)
			return
		}

		rules := store.ListRules(claims.Tenant)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"rules": rules,
			"count": len(rules),
		})
	}))))

	mux.Handle("POST /api/v1/policies", authMiddleware.RequireAuth(authMiddleware.RequireTenant(authMiddleware.RequireScope("policy:write")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			http.Error(w, `{"error": "no claims"}`, http.StatusInternalServerError)
			return
		}

		var rule PolicyRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
			return
		}

		// Override tenant with the verified token tenant
		rule.Tenant = claims.Tenant

		created, _ := store.CreateRule(&rule)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(created)
	})))))

	mux.Handle("GET /api/v1/policies/{id}", authMiddleware.RequireAuth(authMiddleware.RequireTenant(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			http.Error(w, `{"error": "no claims"}`, http.StatusInternalServerError)
			return
		}

		id := r.PathValue("id")
		rule, ok := store.GetRule(id)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
			return
		}

		// Verify tenant access
		if rule.Tenant != "" && rule.Tenant != claims.Tenant {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "access denied"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(rule)
	}))))

	mux.Handle("DELETE /api/v1/policies/{id}", authMiddleware.RequireAuth(authMiddleware.RequireTenant(authMiddleware.RequireScope("policy:write")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			http.Error(w, `{"error": "no claims"}`, http.StatusInternalServerError)
			return
		}

		id := r.PathValue("id")
		rule, ok := store.GetRule(id)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
			return
		}

		// Verify tenant access
		if rule.Tenant != "" && rule.Tenant != claims.Tenant {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "access denied"})
			return
		}

		if !store.DeleteRule(id) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})))))

	mux.Handle("POST /api/v1/evaluate", authMiddleware.RequireAuth(authMiddleware.RequireTenant(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		decision := store.Evaluate(req.ResourceID, req.UserRole, req.RequestedScope, req.Region, req.Labels)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(decision)
	}))))

	mux.Handle("POST /api/v1/batch-evaluate", authMiddleware.RequireAuth(authMiddleware.RequireTenant(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Requests []struct {
				ResourceID    string   `json:"resourceId"`
				UserRole      string   `json:"userRole"`
				RequestedScope string   `json:"requestedScope"`
				Region        string   `json:"region"`
				Labels        []string `json:"labels"`
			} `json:"requests"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
			return
		}
		var decisions []*PolicyDecision
		for _, r := range req.Requests {
			decision := store.Evaluate(r.ResourceID, r.UserRole, r.RequestedScope, r.Region, r.Labels)
			decisions = append(decisions, decision)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"decisions": decisions,
		})
	}))))

	return mux
}
