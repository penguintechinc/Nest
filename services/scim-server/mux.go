package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"go.uber.org/zap"
)

func NewMux(store *SCIMStore, logger *zap.Logger) http.Handler {
	mux := http.NewServeMux()

	// Health endpoint
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	// SCIM endpoints - protected by license
	mux.HandleFunc("GET /scim/v2/Users", scimListUsersHandler(store, logger))
	mux.HandleFunc("POST /scim/v2/Users", scimCreateUserHandler(store, logger))
	mux.HandleFunc("GET /scim/v2/Users/{id}", scimGetUserHandler(store, logger))
	mux.HandleFunc("PUT /scim/v2/Users/{id}", scimUpdateUserHandler(store, logger))
	mux.HandleFunc("DELETE /scim/v2/Users/{id}", scimDeleteUserHandler(store, logger))

	mux.HandleFunc("GET /scim/v2/Groups", scimListGroupsHandler(store, logger))
	mux.HandleFunc("POST /scim/v2/Groups", scimCreateGroupHandler(store, logger))
	mux.HandleFunc("GET /scim/v2/Groups/{id}", scimGetGroupHandler(store, logger))
	mux.HandleFunc("DELETE /scim/v2/Groups/{id}", scimDeleteGroupHandler(store, logger))

	mux.HandleFunc("GET /scim/v2/ServiceProviderConfig", scimServiceProviderConfigHandler(logger))

	return mux
}

func requireEnterpriseLicense(w http.ResponseWriter, logger *zap.Logger) bool {
	if os.Getenv("ENTERPRISE_LICENSE") == "" {
		w.Header().Set("Content-Type", "application/scim+json")
		w.WriteHeader(http.StatusPaymentRequired)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "enterprise license required",
		})
		logger.Warn("license check failed")
		return false
	}
	return true
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
		if !requireEnterpriseLicense(w, logger) {
			return
		}

		users := store.ListUsers()
		response := map[string]interface{}{
			"schemas":      []string{"urn:ietf:params:scim:api:messages:2.0:ListResponse"},
			"totalResults": len(users),
			"startIndex":   1,
			"itemsPerPage": len(users),
			"Resources":    users,
		}

		logger.Info("users listed", zap.Int("count", len(users)))
		writeSCIMJSON(w, http.StatusOK, response)
	}
}

func scimCreateUserHandler(store *SCIMStore, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEnterpriseLicense(w, logger) {
			return
		}

		var user SCIMUser
		if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
			writeSCIMError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if user.UserName == "" {
			writeSCIMError(w, http.StatusBadRequest, "userName is required")
			return
		}

		created := store.CreateUser(&user)

		// Add schemas to response
		created.Meta.ResourceType = "User"
		response := created
		responseDump := map[string]interface{}{
			"schemas":     []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
			"id":          response.ID,
			"userName":    response.UserName,
			"displayName": response.DisplayName,
			"emails":      response.Emails,
			"active":      response.Active,
			"groups":      response.Groups,
			"meta":        response.Meta,
		}

		w.Header().Set("Location", fmt.Sprintf("/scim/v2/Users/%s", created.ID))
		logger.Info("user created", zap.String("id", created.ID), zap.String("userName", created.UserName))
		writeSCIMJSON(w, http.StatusCreated, responseDump)
	}
}

func scimGetUserHandler(store *SCIMStore, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEnterpriseLicense(w, logger) {
			return
		}

		id := r.PathValue("id")
		user, err := store.GetUser(id)
		if err != nil {
			writeSCIMError(w, http.StatusNotFound, "user not found")
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
		if !requireEnterpriseLicense(w, logger) {
			return
		}

		id := r.PathValue("id")

		var user SCIMUser
		if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
			writeSCIMError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		updated, err := store.UpdateUser(id, &user)
		if err != nil {
			writeSCIMError(w, http.StatusNotFound, "user not found")
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
		if !requireEnterpriseLicense(w, logger) {
			return
		}

		id := r.PathValue("id")
		if err := store.DeleteUser(id); err != nil {
			writeSCIMError(w, http.StatusNotFound, "user not found")
			return
		}

		logger.Info("user deleted", zap.String("id", id))
		w.WriteHeader(http.StatusNoContent)
	}
}

// Group Handlers

func scimListGroupsHandler(store *SCIMStore, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEnterpriseLicense(w, logger) {
			return
		}

		groups := store.ListGroups()
		response := map[string]interface{}{
			"schemas":      []string{"urn:ietf:params:scim:api:messages:2.0:ListResponse"},
			"totalResults": len(groups),
			"startIndex":   1,
			"itemsPerPage": len(groups),
			"Resources":    groups,
		}

		logger.Info("groups listed", zap.Int("count", len(groups)))
		writeSCIMJSON(w, http.StatusOK, response)
	}
}

func scimCreateGroupHandler(store *SCIMStore, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEnterpriseLicense(w, logger) {
			return
		}

		var group SCIMGroup
		if err := json.NewDecoder(r.Body).Decode(&group); err != nil {
			writeSCIMError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if group.DisplayName == "" {
			writeSCIMError(w, http.StatusBadRequest, "displayName is required")
			return
		}

		created := store.CreateGroup(&group)

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
		if !requireEnterpriseLicense(w, logger) {
			return
		}

		id := r.PathValue("id")
		group, err := store.GetGroup(id)
		if err != nil {
			writeSCIMError(w, http.StatusNotFound, "group not found")
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
		if !requireEnterpriseLicense(w, logger) {
			return
		}

		id := r.PathValue("id")
		if err := store.DeleteGroup(id); err != nil {
			writeSCIMError(w, http.StatusNotFound, "group not found")
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
