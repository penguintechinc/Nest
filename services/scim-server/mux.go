package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/penguintechinc/nest/shared/go_libs/auth"
	"github.com/penguintechinc/nest/shared/go_libs/http/middleware"
	"github.com/penguintechinc/nest/shared/licensing"
	"go.uber.org/zap"
)

func NewMux(store *SCIMStore, logger *zap.Logger) http.Handler {
	mux := http.NewServeMux()

	validator := licensing.NewValidator(os.Getenv("ENTERPRISE_LICENSE"), "nest")
	jwksURL := os.Getenv("OIDC_JWKS_URL")
	if jwksURL == "" {
		logger.Warn("OIDC_JWKS_URL not set; auth will fail unless in test mode")
	}

	authMiddleware := middleware.AuthMiddleware(jwksURL, logger)

	// Health endpoint
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	// Protected routes wrapper
	protected := func(next http.HandlerFunc) http.Handler {
		return authMiddleware(validator.Middleware(middleware.TenantFilter(next)))
	}

	// SCIM endpoints - protected by license and auth
	mux.Handle("GET /scim/v2/Users", protected(scimListUsersHandler(store, logger)))
	mux.Handle("POST /scim/v2/Users", protected(scimCreateUserHandler(store, logger)))
	mux.Handle("GET /scim/v2/Users/{id}", protected(scimGetUserHandler(store, logger)))
	mux.Handle("PUT /scim/v2/Users/{id}", protected(scimUpdateUserHandler(store, logger)))
	mux.Handle("DELETE /scim/v2/Users/{id}", protected(scimDeleteUserHandler(store, logger)))

	mux.Handle("GET /scim/v2/Groups", protected(scimListGroupsHandler(store, logger)))
	mux.Handle("POST /scim/v2/Groups", protected(scimCreateGroupHandler(store, logger)))
	mux.Handle("GET /scim/v2/Groups/{id}", protected(scimGetGroupHandler(store, logger)))
	mux.Handle("DELETE /scim/v2/Groups/{id}", protected(scimDeleteGroupHandler(store, logger)))

	mux.HandleFunc("GET /scim/v2/ServiceProviderConfig", scimServiceProviderConfigHandler(logger))

	// Catch-all
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/healthz") &&
			!strings.HasPrefix(r.URL.Path, "/scim/v2/") {
			w.Header().Set("Content-Type", "application/scim+json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
			return
		}
		mux.ServeHTTP(w, r)
	})

	return handler
}

func writeSCIMJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/scim+json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeSCIMError(w http.ResponseWriter, status int, msg string) {
	writeSCIMJSON(w, status, map[string]string{"error": msg})
}

// User Handlers

func scimListUsersHandler(store *SCIMStore, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		users := store.ListUsers(cl.Tenant)
		response := map[string]interface{}{
			"schemas":      []string{"urn:ietf:params:scim:api:messages:2.0:ListResponse"},
			"totalResults": len(users),
			"startIndex":   1,
			"itemsPerPage": len(users),
			"Resources":    users,
		}

		logger.Info("users listed", zap.Int("count", len(users)), zap.String("tenant", cl.Tenant))
		writeSCIMJSON(w, http.StatusOK, response)
	}
}

func scimCreateUserHandler(store *SCIMStore, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		var user SCIMUser
		if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
			writeSCIMError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if user.UserName == "" {
			writeSCIMError(w, http.StatusBadRequest, "userName is required")
			return
		}

		created := store.CreateUser(cl.Tenant, &user)
		if created == nil {
			writeSCIMError(w, http.StatusInternalServerError, "failed to create user")
			return
		}

		// Add schemas to response
		created.Meta.ResourceType = "User"
		responseDump := map[string]interface{}{
			"schemas":     []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
			"id":          created.ID,
			"userName":    created.UserName,
			"displayName": created.DisplayName,
			"emails":      created.Emails,
			"active":      created.Active,
			"groups":      created.Groups,
			"meta":        created.Meta,
		}

		w.Header().Set("Location", fmt.Sprintf("/scim/v2/Users/%s", created.ID))
		logger.Info("user created", zap.String("id", created.ID), zap.String("userName", created.UserName))
		writeSCIMJSON(w, http.StatusCreated, responseDump)
	}
}

func scimGetUserHandler(store *SCIMStore, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		id := r.PathValue("id")
		user, err := store.GetUser(cl.Tenant, id)
		if err != nil {
			writeSCIMError(w, http.StatusNotFound, "user not found or unauthorized")
			return
		}

		user.Meta.ResourceType = "User"
		response := map[string]interface{}{
			"schemas":     []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
			"id":          user.ID,
			"userName":    user.UserName,
			"displayName": user.DisplayName,
			"emails":      user.Emails,
			"active":      user.Active,
			"groups":      user.Groups,
			"meta":        user.Meta,
		}

		logger.Info("user retrieved", zap.String("id", id))
		writeSCIMJSON(w, http.StatusOK, response)
	}
}

func scimUpdateUserHandler(store *SCIMStore, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		id := r.PathValue("id")

		var user SCIMUser
		if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
			writeSCIMError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		updated, err := store.UpdateUser(cl.Tenant, id, &user)
		if err != nil {
			writeSCIMError(w, http.StatusNotFound, "user not found or unauthorized")
			return
		}

		updated.Meta.ResourceType = "User"
		response := map[string]interface{}{
			"schemas":     []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
			"id":          updated.ID,
			"userName":    updated.UserName,
			"displayName": updated.DisplayName,
			"emails":      updated.Emails,
			"active":      updated.Active,
			"groups":      updated.Groups,
			"meta":        updated.Meta,
		}

		logger.Info("user updated", zap.String("id", id))
		writeSCIMJSON(w, http.StatusOK, response)
	}
}

func scimDeleteUserHandler(store *SCIMStore, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		id := r.PathValue("id")
		if err := store.DeleteUser(cl.Tenant, id); err != nil {
			writeSCIMError(w, http.StatusNotFound, "user not found or unauthorized")
			return
		}

		logger.Info("user deleted", zap.String("id", id))
		w.WriteHeader(http.StatusNoContent)
	}
}

// Group Handlers

func scimListGroupsHandler(store *SCIMStore, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		groups := store.ListGroups(cl.Tenant)
		response := map[string]interface{}{
			"schemas":      []string{"urn:ietf:params:scim:api:messages:2.0:ListResponse"},
			"totalResults": len(groups),
			"startIndex":   1,
			"itemsPerPage": len(groups),
			"Resources":    groups,
		}

		logger.Info("groups listed", zap.Int("count", len(groups)), zap.String("tenant", cl.Tenant))
		writeSCIMJSON(w, http.StatusOK, response)
	}
}

func scimCreateGroupHandler(store *SCIMStore, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		var group SCIMGroup
		if err := json.NewDecoder(r.Body).Decode(&group); err != nil {
			writeSCIMError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if group.DisplayName == "" {
			writeSCIMError(w, http.StatusBadRequest, "displayName is required")
			return
		}

		created := store.CreateGroup(cl.Tenant, &group)
		if created == nil {
			writeSCIMError(w, http.StatusInternalServerError, "failed to create group")
			return
		}

		created.Meta.ResourceType = "Group"
		response := map[string]interface{}{
			"schemas":     []string{"urn:ietf:params:scim:schemas:core:2.0:Group"},
			"id":          created.ID,
			"displayName": created.DisplayName,
			"members":     created.Members,
			"meta":        created.Meta,
		}

		w.Header().Set("Location", fmt.Sprintf("/scim/v2/Groups/%s", created.ID))
		logger.Info("group created", zap.String("id", created.ID), zap.String("displayName", created.DisplayName))
		writeSCIMJSON(w, http.StatusCreated, response)
	}
}

func scimGetGroupHandler(store *SCIMStore, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		id := r.PathValue("id")
		group, err := store.GetGroup(cl.Tenant, id)
		if err != nil {
			writeSCIMError(w, http.StatusNotFound, "group not found or unauthorized")
			return
		}

		group.Meta.ResourceType = "Group"
		response := map[string]interface{}{
			"schemas":     []string{"urn:ietf:params:scim:schemas:core:2.0:Group"},
			"id":          group.ID,
			"displayName": group.DisplayName,
			"members":     group.Members,
			"meta":        group.Meta,
		}

		logger.Info("group retrieved", zap.String("id", id))
		writeSCIMJSON(w, http.StatusOK, response)
	}
}

func scimDeleteGroupHandler(store *SCIMStore, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cl, _ := auth.FromContext(r.Context())
		id := r.PathValue("id")
		if err := store.DeleteGroup(cl.Tenant, id); err != nil {
			writeSCIMError(w, http.StatusNotFound, "group not found or unauthorized")
			return
		}

		logger.Info("group deleted", zap.String("id", id))
		w.WriteHeader(http.StatusNoContent)
	}
}

// Service Provider Config

func scimServiceProviderConfigHandler(logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		config := map[string]interface{}{
			"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"},
			"patch": map[string]interface{}{
				"supported": true,
			},
			"bulk": map[string]interface{}{
				"supported": false,
			},
			"filter": map[string]interface{}{
				"supported": false,
			},
			"changePassword": map[string]interface{}{
				"supported": true,
			},
			"sort": map[string]interface{}{
				"supported": false,
			},
			"etag": map[string]interface{}{
				"supported": true,
			},
			"authenticationSchemes": []map[string]interface{}{
				{
					"name":        "OAuth Bearer Token",
					"description": "Authentication via HTTP Bearer Token",
					"specUri":     "http://www.rfc-editor.org/info/rfc6750",
					"type":        "oauthbearertoken",
					"primary":     true,
				},
			},
		}

		logger.Info("service provider config retrieved")
		writeSCIMJSON(w, http.StatusOK, config)
	}
}
