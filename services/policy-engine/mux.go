package main

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"
)

func NewMux(store *PolicyStore, logger *zap.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /api/v1/policies", func(w http.ResponseWriter, r *http.Request) {
		tenant := r.URL.Query().Get("tenant")
		rules := store.ListRules(tenant)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"rules": rules,
			"count": len(rules),
		})
	})

	mux.HandleFunc("POST /api/v1/policies", func(w http.ResponseWriter, r *http.Request) {
		var rule PolicyRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
			return
		}
		created, _ := store.CreateRule(&rule)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(created)
	})

	mux.HandleFunc("GET /api/v1/policies/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		rule, ok := store.GetRule(id)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(rule)
	})

	mux.HandleFunc("DELETE /api/v1/policies/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !store.DeleteRule(id) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/v1/evaluate", func(w http.ResponseWriter, r *http.Request) {
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
	})

	mux.HandleFunc("POST /api/v1/batch-evaluate", func(w http.ResponseWriter, r *http.Request) {
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
	})

	return mux
}
