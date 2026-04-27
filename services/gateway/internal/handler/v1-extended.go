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

type createExtendedRequest struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Class string `json:"class"`
}

func extListHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
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

		logger.Info("extended engine list", zap.String("tenant", tid))

		if cfg.K8sClient == nil {
			writeError(w, http.StatusServiceUnavailable, "k8s client unavailable")
			return
		}

		var list nestv1.DataResourceList
		if err := cfg.K8sClient.List(r.Context(), &list,
			client.InNamespace(tid),
			client.MatchingLabels{"nest.penguintech.io/tenant": tid},
		); err != nil {
			logger.Error("k8s list extended engine DataResources", zap.String("tenant", tid), zap.Error(err))
			writeError(w, http.StatusInternalServerError, "failed to list engines")
			return
		}

		items := make([]dataresourceResponse, 0)
		for _, dr := range list.Items {
			if nestv1.IsExtendedType(dr.Spec.Type) {
				items = append(items, toDataresourceResponse(dr))
			}
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"engines": items,
			"count":   len(items),
			"tenant":  tid,
		})
	}
}

func extGetHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
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

		logger.Info("extended engine get", zap.String("tenant", tid), zap.String("name", name))

		if cfg.K8sClient == nil {
			writeError(w, http.StatusServiceUnavailable, "k8s client unavailable")
			return
		}

		var dr nestv1.DataResource
		if err := cfg.K8sClient.Get(r.Context(), types.NamespacedName{Namespace: tid, Name: name}, &dr); err != nil {
			if isNotFound(err) {
				writeError(w, http.StatusNotFound, "engine not found")
				return
			}
			logger.Error("k8s get extended engine DataResource", zap.String("tenant", tid), zap.String("name", name), zap.Error(err))
			writeError(w, http.StatusInternalServerError, "failed to get engine")
			return
		}

		if dr.Spec.Tenant != tid {
			writeError(w, http.StatusForbidden, "tenant mismatch on resource")
			return
		}
		if !nestv1.IsExtendedType(dr.Spec.Type) {
			writeError(w, http.StatusNotFound, "engine not found")
			return
		}

		writeJSON(w, http.StatusOK, toDataresourceResponse(dr))
	}
}

func extCreateHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
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

		var req createExtendedRequest
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
		if !nestv1.IsExtendedType(req.Type) {
			writeError(w, http.StatusBadRequest, "type is not an extended engine type")
			return
		}

		logger.Info("extended engine create",
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
				Origination: nestv1.OriginationManaged,
			},
		}

		if err := cfg.K8sClient.Create(r.Context(), dr); err != nil {
			if isAlreadyExists(err) {
				writeError(w, http.StatusConflict, "engine already exists")
				return
			}
			logger.Error("k8s create extended engine DataResource", zap.String("tenant", tid), zap.String("name", req.Name), zap.Error(err))
			writeError(w, http.StatusInternalServerError, "failed to create engine")
			return
		}

		w.Header().Set("Location", "/api/v1/tenants/"+tid+"/engines/"+req.Name)
		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"status": "accepted",
			"engine": toDataresourceResponse(*dr),
		})
	}
}

func extDeleteHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
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

		logger.Info("extended engine delete",
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
				writeError(w, http.StatusNotFound, "engine not found")
				return
			}
			logger.Error("k8s get extended engine DataResource for delete", zap.String("tenant", tid), zap.String("name", name), zap.Error(err))
			writeError(w, http.StatusInternalServerError, "failed to delete engine")
			return
		}

		if dr.Spec.Tenant != tid {
			writeError(w, http.StatusForbidden, "tenant mismatch on resource")
			return
		}
		if !nestv1.IsExtendedType(dr.Spec.Type) {
			writeError(w, http.StatusNotFound, "engine not found")
			return
		}

		if err := cfg.K8sClient.Delete(r.Context(), &dr); err != nil {
			if isNotFound(err) {
				writeError(w, http.StatusNotFound, "engine not found")
				return
			}
			logger.Error("k8s delete extended engine DataResource", zap.String("tenant", tid), zap.String("name", name), zap.Error(err))
			writeError(w, http.StatusInternalServerError, "failed to delete engine")
			return
		}

		writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}
