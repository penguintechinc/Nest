package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/penguintechinc/nest/pkg/auth"
	"go.uber.org/zap"
)

// NewMux creates an HTTP router for the audit service.
func NewMux(auditLogger *AuditLogger, enterpriseLicense string, logger *zap.Logger, authMiddleware *auth.Middleware) *http.ServeMux {
	mux := http.NewServeMux()

	// Health check endpoint (no license required)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Middleware to check license for protected endpoints
	requireLicense := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if enterpriseLicense == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusPaymentRequired) // 402
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error": "enterprise license required",
					"code":  "nest.enterprise.license_required",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	// POST /api/v1/audit/events - append event (SECURITY: requires auth + tenant)
	postHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var event AuditEvent
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
			return
		}

		// Extract claims from verified token (SECURITY: never trust body-supplied actor/tenant)
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			http.Error(w, fmt.Sprintf(`{"error": "no claims in context"}`), http.StatusInternalServerError)
			return
		}

		// SECURITY: Override actor and tenant from verified JWT claims
		event.Actor = claims.Sub     // subject/user_uuid from token
		event.Tenant = claims.Tenant // tenant from token

		// Set timestamp from server clock (ignore client-supplied value)
		event.Timestamp = time.Now()

		if err := auditLogger.Append(&event); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(event)
	})
	mux.Handle("POST /api/v1/audit/events", requireLicense(authMiddleware.RequireAuth(authMiddleware.RequireTenant(postHandler))))

	// GET /api/v1/audit/events - query events (SECURITY: tenant-scoped, no IDOR)
	getEventsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract claims from verified token
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			http.Error(w, fmt.Sprintf(`{"error": "no claims in context"}`), http.StatusInternalServerError)
			return
		}

		// Parse query parameters
		filter := AuditFilter{
			Tenant:   claims.Tenant, // SECURITY: always use token's tenant, never trust query param
			Actor:    r.URL.Query().Get("actor"),
			Action:   r.URL.Query().Get("action"),
			Resource: r.URL.Query().Get("resource"),
			Outcome:  r.URL.Query().Get("outcome"),
		}

		// Parse start_time and end_time as RFC3339
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
	})
	mux.Handle("GET /api/v1/audit/events", requireLicense(authMiddleware.RequireAuth(authMiddleware.RequireTenant(getEventsHandler))))

	// GET /api/v1/audit/events/{id} - get single event (SECURITY: tenant-scoped)
	getEventHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "missing event id"})
			return
		}

		// Extract claims from verified token
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			http.Error(w, fmt.Sprintf(`{"error": "no claims in context"}`), http.StatusInternalServerError)
			return
		}

		// Query for event with specific ID, scoped to token's tenant (SECURITY: prevent IDOR)
		events := auditLogger.Query(AuditFilter{Tenant: claims.Tenant})
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
			json.NewEncoder(w).Encode(map[string]string{"error": "event not found"})
			return
		}

		// Additional safety check: verify found event belongs to requester's tenant
		if found.Tenant != claims.Tenant {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "access denied"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(found)
	})
	mux.Handle("GET /api/v1/audit/events/{id}", requireLicense(authMiddleware.RequireAuth(authMiddleware.RequireTenant(getEventHandler))))

	// GET /api/v1/audit/verify - validate chain integrity
	verifyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		err := auditLogger.ValidateIntegrity(ctx)
		w.Header().Set("Content-Type", "application/json")

		if err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "invalid",
				"error":  err.Error(),
			})
		} else {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "valid",
			})
		}
	})
	mux.Handle("GET /api/v1/audit/verify", requireLicense(authMiddleware.RequireAuth(verifyHandler)))

	// Catch-all for 404
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Only log unrecognized paths
		if !strings.HasPrefix(r.URL.Path, "/healthz") &&
			!strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		}
	})

	return mux
}
