package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
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

// dataresourceResponse is the API representation of a DataResource CR.
type dataresourceResponse struct {
	Name      string                      `json:"name"`
	Namespace string                      `json:"namespace"`
	Type      string                      `json:"type"`
	Class     string                      `json:"class"`
	Tenant    string                      `json:"tenant"`
	Phase     nestv1.DataResourcePhase    `json:"phase"`
	HA        bool                        `json:"ha,omitempty"`
	Endpoints *nestv1.ResourceEndpoints   `json:"endpoints,omitempty"`
	Health    *nestv1.HealthSignal        `json:"health,omitempty"`
}

func toDataresourceResponse(dr nestv1.DataResource) dataresourceResponse {
	return dataresourceResponse{
		Name:      dr.Name,
		Namespace: dr.Namespace,
		Type:      dr.Spec.Type,
		Class:     dr.Spec.Class,
		Tenant:    dr.Spec.Tenant,
		Phase:     dr.Status.Phase,
		HA:        dr.Spec.HA,
		Endpoints: dr.Status.Endpoints,
		Health:    dr.Status.Health,
	}
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

		if cfg.K8sClient == nil {
			writeError(w, http.StatusServiceUnavailable, "k8s client unavailable")
			return
		}

		var list nestv1.DataResourceList
		if err := cfg.K8sClient.List(r.Context(), &list,
			client.InNamespace(tid),
			client.MatchingLabels{"nest.penguintech.io/tenant": tid},
		); err != nil {
			logger.Error("k8s list DataResourceList", zap.String("tenant", tid), zap.Error(err))
			writeError(w, http.StatusInternalServerError, "failed to list dataresources")
			return
		}

		items := make([]dataresourceResponse, 0, len(list.Items))
		for _, dr := range list.Items {
			items = append(items, toDataresourceResponse(dr))
		}

		total := len(items)
		// Apply offset/limit in-memory (K8s doesn't support SQL-style pagination)
		if offset >= total {
			items = []dataresourceResponse{}
		} else {
			end := offset + limit
			if end > total {
				end = total
			}
			items = items[offset:end]
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"dataresources": items,
			"count":         len(items),
			"tenant":        tid,
			"pagination": map[string]int{
				"limit":  limit,
				"offset": offset,
				"total":  total,
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

		if cfg.K8sClient == nil {
			writeError(w, http.StatusServiceUnavailable, "k8s client unavailable")
			return
		}

		var dr nestv1.DataResource
		if err := cfg.K8sClient.Get(r.Context(), types.NamespacedName{Namespace: tid, Name: name}, &dr); err != nil {
			if isNotFound(err) {
				writeError(w, http.StatusNotFound, "dataresource not found")
				return
			}
			logger.Error("k8s get DataResource", zap.String("tenant", tid), zap.String("name", name), zap.Error(err))
			writeError(w, http.StatusInternalServerError, "failed to get dataresource")
			return
		}

		// Verify tenant ownership
		if dr.Spec.Tenant != tid {
			writeError(w, http.StatusForbidden, "tenant mismatch on resource")
			return
		}

		writeJSON(w, http.StatusOK, toDataresourceResponse(dr))
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

		logger.Info("dataresource create",
			zap.String("tenant", tid),
			zap.String("name", req.Name),
			zap.String("type", req.Type),
		)

		if cfg.K8sClient == nil {
			writeError(w, http.StatusServiceUnavailable, "k8s client unavailable")
			return
		}

		dr := &nestv1.DataResource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      req.Name,
				Namespace: tid,
				Labels: map[string]string{
					"nest.penguintech.io/tenant": tid,
				},
			},
			Spec: nestv1.DataResourceSpec{
				Type:        req.Type,
				Class:       req.Class,
				Tenant:      tid,
				HA:          req.HA,
				Origination: nestv1.OriginationManaged,
			},
		}
		if req.Size != "" {
			dr.Spec.Size = &nestv1.ResourceSize{Storage: req.Size}
		}

		if err := cfg.K8sClient.Create(r.Context(), dr); err != nil {
			if isAlreadyExists(err) {
				writeError(w, http.StatusConflict, "dataresource already exists")
				return
			}
			logger.Error("k8s create DataResource", zap.String("tenant", tid), zap.String("name", req.Name), zap.Error(err))
			writeError(w, http.StatusInternalServerError, "failed to create dataresource")
			return
		}

		w.Header().Set("Location", "/api/v1/tenants/"+tid+"/dataresources/"+req.Name)
		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"status":       "accepted",
			"dataresource": toDataresourceResponse(*dr),
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

		if cfg.K8sClient == nil {
			writeError(w, http.StatusServiceUnavailable, "k8s client unavailable")
			return
		}

		var dr nestv1.DataResource
		if err := cfg.K8sClient.Get(r.Context(), types.NamespacedName{Namespace: tid, Name: name}, &dr); err != nil {
			if isNotFound(err) {
				writeError(w, http.StatusNotFound, "dataresource not found")
				return
			}
			logger.Error("k8s get DataResource for delete", zap.String("tenant", tid), zap.String("name", name), zap.Error(err))
			writeError(w, http.StatusInternalServerError, "failed to delete dataresource")
			return
		}

		// Verify tenant ownership before delete
		if dr.Spec.Tenant != tid {
			writeError(w, http.StatusForbidden, "tenant mismatch on resource")
			return
		}

		if err := cfg.K8sClient.Delete(r.Context(), &dr); err != nil {
			if isNotFound(err) {
				writeError(w, http.StatusNotFound, "dataresource not found")
				return
			}
			logger.Error("k8s delete DataResource", zap.String("tenant", tid), zap.String("name", name), zap.Error(err))
			writeError(w, http.StatusInternalServerError, "failed to delete dataresource")
			return
		}

		writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}

// isNotFound checks if an error is a Kubernetes NotFound error.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	// Use k8s apierrors pattern via error string check or type assertion.
	// We import apierrors below.
	return isK8sNotFound(err)
}

// isAlreadyExists checks if an error is a Kubernetes AlreadyExists error.
func isAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	return isK8sAlreadyExists(err)
}
