package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/penguintechinc/nest/pkg/auth"
	"go.uber.org/zap"
)

// validLocation checks if a location is safe (no path traversal, valid S3/file path).
func validLocation(location string) bool {
	if location == "" {
		return false
	}
	// Reject path traversal attempts
	if strings.Contains(location, "..") {
		return false
	}
	// Must be a valid URI-like path (s3://, file://, etc.)
	return strings.Contains(location, "://")
}

// NewMux creates the HTTP handler for the Iceberg REST API.
func NewMux(catalog *Catalog, logger *zap.Logger, authMiddleware *auth.Middleware) http.Handler {
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Metrics stub
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("# HELP iceberg_catalog_namespaces Total namespaces\n# TYPE iceberg_catalog_namespaces gauge\niceberg_catalog_namespaces 0\n"))
	})

	// Namespace routes
	mux.Handle("GET /v1/namespaces", authMiddleware.RequireAuth(authMiddleware.RequireTenant(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			writeError(w, http.StatusInternalServerError, "no claims")
			return
		}

		tenant := claims.Tenant
		namespaces := catalog.ListNamespaces("")
		var result [][]string
		for _, ns := range namespaces {
			// Filter by token tenant
			if !strings.HasPrefix(ns.Name, tenant+".") && ns.Name != tenant {
				continue
			}
			result = append(result, []string{ns.Name})
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"namespaces": result})
	}))))

	mux.Handle("POST /v1/namespaces", authMiddleware.RequireAuth(authMiddleware.RequireTenant(authMiddleware.RequireScope("iceberg:write")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			writeError(w, http.StatusInternalServerError, "no claims")
			return
		}

		tenant := claims.Tenant
		var req struct {
			Namespace  []string          `json:"namespace"`
			Properties map[string]string `json:"properties"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if len(req.Namespace) == 0 {
			writeError(w, http.StatusBadRequest, "namespace is required")
			return
		}

		nsName := strings.Join(req.Namespace, ".")
		// Tenant isolation check
		if !strings.HasPrefix(nsName, tenant+".") && nsName != tenant {
			writeError(w, http.StatusForbidden, "tenant mismatch")
			return
		}

		ns, err := catalog.CreateNamespace(nsName, req.Properties)
		if err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"namespace":  req.Namespace,
			"properties": ns.Properties,
		})
	})))))

	mux.Handle("GET /v1/namespaces/{namespace}", authMiddleware.RequireAuth(authMiddleware.RequireTenant(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			writeError(w, http.StatusInternalServerError, "no claims")
			return
		}

		tenant := claims.Tenant
		nsName := r.PathValue("namespace")
		nsName = strings.ReplaceAll(nsName, "/", ".")

		// Tenant isolation check
		if !strings.HasPrefix(nsName, tenant+".") && nsName != tenant {
			writeError(w, http.StatusForbidden, "tenant mismatch")
			return
		}

		ns, err := catalog.GetNamespace(nsName)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}

		nsParts := strings.Split(nsName, ".")
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"namespace":  nsParts,
			"properties": ns.Properties,
		})
	}))))

	mux.Handle("DELETE /v1/namespaces/{namespace}", authMiddleware.RequireAuth(authMiddleware.RequireTenant(authMiddleware.RequireScope("iceberg:write")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			writeError(w, http.StatusInternalServerError, "no claims")
			return
		}

		tenant := claims.Tenant
		nsName := r.PathValue("namespace")
		nsName = strings.ReplaceAll(nsName, "/", ".")

		// Tenant isolation check
		if !strings.HasPrefix(nsName, tenant+".") && nsName != tenant {
			writeError(w, http.StatusForbidden, "tenant mismatch")
			return
		}

		if err := catalog.DropNamespace(nsName); err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
			} else if strings.Contains(err.Error(), "not empty") {
				writeError(w, http.StatusConflict, err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})))))

	// Table routes
	mux.Handle("GET /v1/namespaces/{namespace}/tables", authMiddleware.RequireAuth(authMiddleware.RequireTenant(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			writeError(w, http.StatusInternalServerError, "no claims")
			return
		}

		tenant := claims.Tenant
		nsName := r.PathValue("namespace")
		nsName = strings.ReplaceAll(nsName, "/", ".")

		// Tenant isolation check
		if !strings.HasPrefix(nsName, tenant+".") && nsName != tenant {
			writeError(w, http.StatusForbidden, "tenant mismatch")
			return
		}

		tables, err := catalog.ListTables(nsName)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}

		var identifiers []map[string]interface{}
		for _, t := range tables {
			identifiers = append(identifiers, map[string]interface{}{
				"namespace": strings.Split(t.Namespace, "."),
				"name":      t.Name,
			})
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"identifiers": identifiers,
		})
	}))))

	mux.Handle("POST /v1/namespaces/{namespace}/tables", authMiddleware.RequireAuth(authMiddleware.RequireTenant(authMiddleware.RequireScope("iceberg:write")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			writeError(w, http.StatusInternalServerError, "no claims")
			return
		}

		tenant := claims.Tenant
		nsName := r.PathValue("namespace")
		nsName = strings.ReplaceAll(nsName, "/", ".")

		// Tenant isolation check
		if !strings.HasPrefix(nsName, tenant+".") && nsName != tenant {
			writeError(w, http.StatusForbidden, "tenant mismatch")
			return
		}

		var req struct {
			Name       string                 `json:"name"`
			Location   string                 `json:"location"`
			Schema     map[string]interface{} `json:"schema"`
			Properties map[string]string      `json:"properties"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if req.Name == "" || req.Location == "" {
			writeError(w, http.StatusBadRequest, "name and location are required")
			return
		}

		// Validate location to prevent path traversal or cross-tenant bucket access
		if !validLocation(req.Location) {
			writeError(w, http.StatusBadRequest, "invalid location: must be a valid URI with no path traversal")
			return
		}

		table, err := catalog.CreateTable(nsName, req.Name, req.Location, req.Schema, req.Properties)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
			} else {
				writeError(w, http.StatusConflict, err.Error())
			}
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"metadata-location": table.MetadataLocation,
			"metadata": map[string]interface{}{
				"table-uuid": table.Name + "-" + table.Namespace,
				"location":   table.Location,
				"schema":     table.Schema,
			},
		})
	})))))

	mux.Handle("GET /v1/namespaces/{namespace}/tables/{table}", authMiddleware.RequireAuth(authMiddleware.RequireTenant(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			writeError(w, http.StatusInternalServerError, "no claims")
			return
		}

		tenant := claims.Tenant
		nsName := r.PathValue("namespace")
		nsName = strings.ReplaceAll(nsName, "/", ".")
		tableName := r.PathValue("table")

		// Tenant isolation check
		if !strings.HasPrefix(nsName, tenant+".") && nsName != tenant {
			writeError(w, http.StatusForbidden, "tenant mismatch")
			return
		}

		table, err := catalog.GetTable(nsName, tableName)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"metadata-location": table.MetadataLocation,
			"metadata": map[string]interface{}{
				"table-uuid": table.Name + "-" + table.Namespace,
				"location":   table.Location,
				"schema":     table.Schema,
			},
		})
	}))))

	mux.Handle("DELETE /v1/namespaces/{namespace}/tables/{table}", authMiddleware.RequireAuth(authMiddleware.RequireTenant(authMiddleware.RequireScope("iceberg:write")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			writeError(w, http.StatusInternalServerError, "no claims")
			return
		}

		tenant := claims.Tenant
		nsName := r.PathValue("namespace")
		nsName = strings.ReplaceAll(nsName, "/", ".")
		tableName := r.PathValue("table")

		// Tenant isolation check
		if !strings.HasPrefix(nsName, tenant+".") && nsName != tenant {
			writeError(w, http.StatusForbidden, "tenant mismatch")
			return
		}

		if err := catalog.DropTable(nsName, tableName); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})))))

	return mux
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
