package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/penguintechinc/nest/services/gateway/internal/claims"
	"github.com/penguintechinc/nest/services/gateway/internal/config"
)

type createDataresourceRequest struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Class string `json:"class"`
	Size  string `json:"size"`
	HA    bool   `json:"ha"`
}

func dataresourceListHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cl, ok := claims.FromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing claims")
			return
		}

		tid := r.PathValue("tid")

		if cl.Tenant != tid {
			writeError(w, http.StatusForbidden, "tenant mismatch")
			return
		}

		// Query params
		limit := 50
		if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
			if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 200 {
				limit = l
			}
		}

		offset := 0
		if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
			if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
				offset = o
			}
		}

		logger.Info("dataresource list",
			zap.String("tenant", tid),
			zap.Int("limit", limit),
			zap.Int("offset", offset),
		)

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"dataresources": []interface{}{},
			"count":         0,
			"tenant":        tid,
			"pagination": map[string]int{
				"limit":  limit,
				"offset": offset,
				"total":  0,
			},
		})
	}
}

func dataresourceGetHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cl, ok := claims.FromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing claims")
			return
		}

		tid := r.PathValue("tid")
		name := r.PathValue("name")

		if cl.Tenant != tid {
			writeError(w, http.StatusForbidden, "tenant mismatch")
			return
		}

		logger.Info("dataresource get",
			zap.String("tenant", tid),
			zap.String("name", name),
		)

		writeError(w, http.StatusNotFound, "dataresource not found")
	}
}

func dataresourceCreateHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cl, ok := claims.FromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing claims")
			return
		}

		tid := r.PathValue("tid")

		if cl.Tenant != tid {
			writeError(w, http.StatusForbidden, "tenant mismatch")
			return
		}

		var req createDataresourceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		if req.Type == "" {
			writeError(w, http.StatusBadRequest, "type is required")
			return
		}

		operationID := uuid.New().String()

		logger.Info("dataresource create",
			zap.String("tenant", tid),
			zap.String("name", req.Name),
			zap.String("type", req.Type),
		)

		w.Header().Set("Location", "/api/v1/tenants/"+tid+"/dataresources/"+req.Name)
		w.Header().Set("X-Operation-ID", operationID)

		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"status":        "accepted",
			"operationId":   operationID,
			"dataresource": map[string]string{
				"name":   req.Name,
				"type":   req.Type,
				"class":  req.Class,
				"tenant": tid,
				"phase":  "pending",
			},
		})
	}
}

func dataresourcePatchHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cl, ok := claims.FromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing claims")
			return
		}

		tid := r.PathValue("tid")
		name := r.PathValue("name")

		if cl.Tenant != tid {
			writeError(w, http.StatusForbidden, "tenant mismatch")
			return
		}

		var body interface{}
		json.NewDecoder(r.Body).Decode(&body)

		logger.Info("dataresource patch",
			zap.String("tenant", tid),
			zap.String("name", name),
		)

		writeJSON(w, http.StatusAccepted, map[string]string{
			"status":  "accepted",
			"message": "update queued",
		})
	}
}

func dataresourceDeleteHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cl, ok := claims.FromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing claims")
			return
		}

		tid := r.PathValue("tid")
		name := r.PathValue("name")

		if cl.Tenant != tid {
			writeError(w, http.StatusForbidden, "tenant mismatch")
			return
		}

		logger.Info("dataresource delete",
			zap.String("tenant", tid),
			zap.String("name", name),
		)

		writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}
