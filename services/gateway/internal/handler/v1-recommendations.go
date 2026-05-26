package handler

import (
	"context"
	"io"
	"net/http"
	"os"
	"time"

	"go.uber.org/zap"

	"github.com/penguintechinc/nest/services/gateway/internal/claims"
	"github.com/penguintechinc/nest/services/gateway/internal/config"
	"github.com/penguintechinc/nest/shared/licensing"
)

// intelligenceRecommendHandler — GET /api/v1/tenants/{tid}/intelligence/recommend
// Enterprise + WaddleAI gated
// Proxies to intelligence-engine at INTELLIGENCE_ENGINE_URL (default http://nest-intelligence-engine:50057)
func intelligenceRecommendHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
	validator := licensing.NewValidator(os.Getenv("ENTERPRISE_LICENSE"), "nest")
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
		if !validator.IsValid(r) || os.Getenv("WADDLEAI_ENABLED") == "" {
			writeJSON(w, http.StatusPaymentRequired, map[string]interface{}{
				"error": "enterprise license required",
				"code":  "nest.enterprise.license_required",
			})
			return
		}

		baseURL := os.Getenv("INTELLIGENCE_ENGINE_URL")
		if baseURL == "" {
			baseURL = "http://nest-intelligence-engine:50057"
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/intelligence/recommendations?tenant="+tid, nil)
		copyHeaders(r, req)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			logger.Warn("upstream unavailable", zap.String("url", baseURL), zap.Error(err))
			writeError(w, http.StatusBadGateway, "upstream unavailable")
			return
		}
		defer resp.Body.Close()

		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}
}

// predictiveDriveHandler — GET /api/v1/tenants/{tid}/predictive-drive/risk
func predictiveDriveHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
	validator := licensing.NewValidator(os.Getenv("ENTERPRISE_LICENSE"), "nest")
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
		if !validator.IsValid(r) || os.Getenv("WADDLEAI_ENABLED") == "" {
			writeJSON(w, http.StatusPaymentRequired, map[string]interface{}{
				"error": "enterprise license required",
				"code":  "nest.enterprise.license_required",
			})
			return
		}

		baseURL := os.Getenv("PREDICTIVE_DRIVE_URL")
		if baseURL == "" {
			baseURL = "http://nest-predictive-drive:50059"
		}

		node := r.URL.Query().Get("node")
		path := "/api/v1/predictive-drive/risk?tenant=" + tid
		if node != "" {
			path += "&node=" + node
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
		copyHeaders(r, req)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			logger.Warn("upstream unavailable", zap.String("url", baseURL), zap.Error(err))
			writeError(w, http.StatusBadGateway, "upstream unavailable")
			return
		}
		defer resp.Body.Close()

		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}
}

// anomalyDetectHandler — GET /api/v1/tenants/{tid}/anomaly/current
func anomalyDetectHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
	validator := licensing.NewValidator(os.Getenv("ENTERPRISE_LICENSE"), "nest")
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
		if !validator.IsValid(r) || os.Getenv("WADDLEAI_ENABLED") == "" {
			writeJSON(w, http.StatusPaymentRequired, map[string]interface{}{
				"error": "enterprise license required",
				"code":  "nest.enterprise.license_required",
			})
			return
		}

		baseURL := os.Getenv("ANOMALY_DETECTOR_URL")
		if baseURL == "" {
			baseURL = "http://nest-anomaly-detector:50061"
		}

		severity := r.URL.Query().Get("severity")
		path := "/api/v1/anomaly/current?tenant=" + tid
		if severity != "" {
			path += "&severity=" + severity
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
		copyHeaders(r, req)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			logger.Warn("upstream unavailable", zap.String("url", baseURL), zap.Error(err))
			writeError(w, http.StatusBadGateway, "upstream unavailable")
			return
		}
		defer resp.Body.Close()

		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}
}
