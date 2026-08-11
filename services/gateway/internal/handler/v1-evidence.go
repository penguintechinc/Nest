package handler

import (
	"net/http"
	"os"
	"time"

	"go.uber.org/zap"

	"github.com/penguintechinc/nest/services/gateway/internal/claims"
	"github.com/penguintechinc/nest/services/gateway/internal/config"
	"github.com/penguintechinc/nest/shared/licensing"
)

func evidenceHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
	validator := licensing.NewValidator(os.Getenv("ENTERPRISE_LICENSE"), "nest")
	return func(w http.ResponseWriter, r *http.Request) {
		cl, ok := claims.FromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing claims")
			return
		}

		if !validator.IsValid(r) {
			writeError(w, http.StatusPaymentRequired, "enterprise license required")
			return
		}

		bundle := r.PathValue("bundle")

		logger.Info("evidence requested",
			zap.String("bundle", bundle),
			zap.String("tenant", cl.Tenant),
		)

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"bundle":      bundle,
			"tenant":      cl.Tenant,
			"generatedAt": time.Now().UTC().Format(time.RFC3339),
			"artifacts": []map[string]interface{}{
				{
					"type":   "audit-log",
					"period": "last-90-days",
					"url":    "/api/v1/audit/events",
				},
				{
					"type":   "policy-check",
					"period": "latest",
					"url":    "/api/v1/policies/compliance",
				},
				{
					"type":   "deployment-manifest",
					"period": "current",
					"url":    "/api/v1/manifests",
				},
			},
			"status": "ready",
		})
	}
}
