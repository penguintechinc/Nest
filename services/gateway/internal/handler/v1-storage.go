package handler

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
	"github.com/penguintechinc/nest/services/gateway/internal/claims"
	"github.com/penguintechinc/nest/services/gateway/internal/config"
)

type createStorageRequest struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	StorageClass string `json:"storageClass"`
	Size         string `json:"size"`
}

func storageListHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
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

		logger.Info("storage list", zap.String("tenant", tid))

		if cfg.K8sClient == nil {
			writeError(w, http.StatusServiceUnavailable, "k8s client unavailable")
			return
		}

		var list nestv1.DataResourceList
		if err := cfg.K8sClient.List(r.Context(), &list,
			client.InNamespace(tid),
			client.MatchingLabels{"nest.penguintech.io/tenant": tid},
		); err != nil {
			logger.Error("k8s list storage DataResources", zap.String("tenant", tid), zap.Error(err))
			writeError(w, http.StatusInternalServerError, "failed to list storage resources")
			return
		}

		items := make([]dataresourceResponse, 0)
		for _, dr := range list.Items {
			if nestv1.IsStorageType(dr.Spec.Type) {
				items = append(items, toDataresourceResponse(dr))
			}
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"resources": items,
			"count":     len(items),
			"tenant":    tid,
		})
	}
}

func storageGetHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
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

		logger.Info("storage get", zap.String("tenant", tid), zap.String("name", name))

		if cfg.K8sClient == nil {
			writeError(w, http.StatusServiceUnavailable, "k8s client unavailable")
			return
		}

		var dr nestv1.DataResource
		if err := cfg.K8sClient.Get(r.Context(), types.NamespacedName{Namespace: tid, Name: name}, &dr); err != nil {
			if isNotFound(err) {
				writeError(w, http.StatusNotFound, "resource not found")
				return
			}
			logger.Error("k8s get storage DataResource", zap.String("tenant", tid), zap.String("name", name), zap.Error(err))
			writeError(w, http.StatusInternalServerError, "failed to get resource")
			return
		}

		if dr.Spec.Tenant != tid {
			writeError(w, http.StatusForbidden, "tenant mismatch on resource")
			return
		}
		if !nestv1.IsStorageType(dr.Spec.Type) {
			writeError(w, http.StatusNotFound, "resource not found")
			return
		}

		writeJSON(w, http.StatusOK, toDataresourceResponse(dr))
	}
}

func storageCreateHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
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

		var req createStorageRequest
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
		if !nestv1.IsStorageType(req.Type) {
			writeError(w, http.StatusBadRequest, "type is not a storage type")
			return
		}

		logger.Info("storage create",
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
				Class:       req.StorageClass,
				Tenant:      tid,
				Origination: nestv1.OriginationManaged,
			},
		}
		if req.Size != "" {
			dr.Spec.Size = &nestv1.ResourceSize{Storage: req.Size}
		}

		if err := cfg.K8sClient.Create(r.Context(), dr); err != nil {
			if isAlreadyExists(err) {
				writeError(w, http.StatusConflict, "resource already exists")
				return
			}
			logger.Error("k8s create storage DataResource", zap.String("tenant", tid), zap.String("name", req.Name), zap.Error(err))
			writeError(w, http.StatusInternalServerError, "failed to create resource")
			return
		}

		w.Header().Set("Location", "/api/v1/tenants/"+tid+"/resources/"+req.Name)
		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"status":   "accepted",
			"resource": toDataresourceResponse(*dr),
		})
	}
}

func storageDeleteHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
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

		logger.Info("storage delete",
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
				writeError(w, http.StatusNotFound, "resource not found")
				return
			}
			logger.Error("k8s get storage DataResource for delete", zap.String("tenant", tid), zap.String("name", name), zap.Error(err))
			writeError(w, http.StatusInternalServerError, "failed to delete resource")
			return
		}

		if dr.Spec.Tenant != tid {
			writeError(w, http.StatusForbidden, "tenant mismatch on resource")
			return
		}
		if !nestv1.IsStorageType(dr.Spec.Type) {
			writeError(w, http.StatusNotFound, "resource not found")
			return
		}

		if err := cfg.K8sClient.Delete(r.Context(), &dr); err != nil {
			if isNotFound(err) {
				writeError(w, http.StatusNotFound, "resource not found")
				return
			}
			logger.Error("k8s delete storage DataResource", zap.String("tenant", tid), zap.String("name", name), zap.Error(err))
			writeError(w, http.StatusInternalServerError, "failed to delete resource")
			return
		}

		writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}
