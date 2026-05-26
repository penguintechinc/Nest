package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/penguintechinc/nest/shared/go_libs/auth"
	"github.com/penguintechinc/nest/shared/go_libs/http/middleware"
	"github.com/penguintechinc/nest/shared/licensing"
	"go.uber.org/zap"
)

// NewMux creates an HTTP router for the audit service.
func NewMux(auditLogger *AuditLogger, enterpriseLicense string, logger *zap.Logger) http.Handler {
	mux := http.NewServeMux()

	validator := licensing.NewValidator(enterpriseLicense, "nest")
	jwksURL := os.Getenv("OIDC_JWKS_URL")
	if jwksURL == "" {
		// In non-prod/non-enterprise, we might allow no auth, 
		// but for world-class we should enforce it.
		// For tests we will set OIDC_JWKS_URL=test
		logger.Warn("OIDC_JWKS_URL not set; auth will fail unless in test mode")
	}

	authMiddleware := middleware.AuthMiddleware(jwksURL, logger)

	// Health check endpoint (no license or auth required)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Middleware to check license
	requireLicense := validator.Middleware

	// Protected routes wrapper
	protected := func(next http.HandlerFunc) http.Handler {
		// Apply Auth -> License -> Tenant Filter
		return authMiddleware(requireLicense(middleware.TenantFilter(next)))
	}

	// POST /api/v1/audit/events - append event
	mux.Handle("POST /api/v1/audit/events", protected(func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		
		var event AuditEvent
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
			return
		}

		// Strictly enforce tenant from claims
		event.Tenant = cl.Tenant
		if event.Actor == "" {
			event.Actor = cl.Subject
		}

		if err := auditLogger.Append(&event); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(event)
	}))

	// GET /api/v1/audit/events - query events
	mux.Handle("GET /api/v1/audit/events", protected(func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())

		// Parse query parameters
		filter := AuditFilter{
			Tenant:   cl.Tenant, // Override with authenticated tenant
			Actor:    r.URL.Query().Get("actor"),
			Action:   r.URL.Query().Get("action"),
			Resource: r.URL.Query().Get("resource"),
			Outcome:  r.URL.Query().Get("outcome"),
		}

		// Parse start_time and end_time
		if startStr := r.URL.Query().Get("start_time"); startStr != "" {
			if t, err := time.Parse(time.RFC3339, startStr); err == nil {
				filter.StartTime = t
			}
		}
		if endStr := r.URL.Query().Get("end_time"); endStr != "" {
			if t, err := time.Parse(time.RFC3339, endStr); err == nil {
				filter.EndTime = t
			}
		}

		// Parse limit and offset
		if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
			if l, err := strconv.Atoi(limitStr); err == nil {
				filter.Limit = l
			}
		}
		if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
			if o, err := strconv.Atoi(offsetStr); err == nil {
				filter.Offset = o
			}
		}

		events := auditLogger.Query(filter)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"events": events,
			"count":  len(events),
		})
	}))

	// GET /api/v1/audit/events/{id} - get single event
	mux.Handle("GET /api/v1/audit/events/{id}", protected(func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		id := r.PathValue("id")
		
		// Query for event and ensure it belongs to the tenant
		events := auditLogger.Query(AuditFilter{Tenant: cl.Tenant})
		var found *AuditEvent
		for _, event := range events {
			if event.ID == id {
				found = event
				break
			}
		}

		if found == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "event not found or unauthorized"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(found)
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
