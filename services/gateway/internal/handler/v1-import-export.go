package handler

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/penguintechinc/nest/services/gateway/internal/claims"
	"github.com/penguintechinc/nest/services/gateway/internal/config"
)

type importRequest struct {
	Source string                 `json:"source"`
	Config map[string]interface{} `json:"config"`
}

type exportRequest struct {
	Destination string                 `json:"destination"`
	Config      map[string]interface{} `json:"config"`
}

func importHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
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

		var req importRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if req.Source == "" {
			writeError(w, http.StatusBadRequest, "source is required")
			return
		}

		operationID := uuid.New().String()

		logger.Info("import requested",
			zap.String("tenant", tid),
			zap.String("resource", name),
			zap.String("source", req.Source),
		)

		w.Header().Set("Location", "/api/v1/tenants/"+tid+"/resources/"+name)
		w.Header().Set("X-Operation-ID", operationID)

		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"status":        "accepted",
			"operationId":   operationID,
			"job": map[string]string{
				"type":     "import",
				"resource": name,
				"tenant":   tid,
				"source":   req.Source,
				"phase":    "pending",
			},
		})
	}
}

func exportHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
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

		var req exportRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if req.Destination == "" {
			writeError(w, http.StatusBadRequest, "destination is required")
			return
		}

		operationID := uuid.New().String()

		logger.Info("export requested",
			zap.String("tenant", tid),
			zap.String("resource", name),
			zap.String("destination", req.Destination),
		)

		w.Header().Set("Location", "/api/v1/tenants/"+tid+"/resources/"+name)
		w.Header().Set("X-Operation-ID", operationID)

		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"status":        "accepted",
			"operationId":   operationID,
			"job": map[string]string{
				"type":        "export",
				"resource":    name,
				"tenant":      tid,
				"destination": req.Destination,
				"phase":       "pending",
			},
		})
	}
}
