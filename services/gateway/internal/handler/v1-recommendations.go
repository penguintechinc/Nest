package handler

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"time"

	"go.uber.org/zap"

	"github.com/penguintechinc/nest/services/gateway/internal/claims"
	"github.com/penguintechinc/nest/services/gateway/internal/config"
)

// validResourceID validates that a resource ID (tenant, node, etc.) is alphanumeric/UUID-safe.
// This prevents SSRF by ensuring only safe characters are used in URL construction.
func validResourceID(id string) bool {
	// Allow alphanumeric, hyphen, and underscore (UUID format is also ok)
	return regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(id)
}

// intelligenceRecommendHandler — GET /api/v1/tenants/{tid}/intelligence/recommend
// Enterprise + WaddleAI gated
// Proxies to intelligence-engine at INTELLIGENCE_ENGINE_URL (default http://nest-intelligence-engine:50057)
func intelligenceRecommendHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
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
		if os.Getenv("ENTERPRISE_LICENSE") == "" || os.Getenv("WADDLEAI_ENABLED") == "" {
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

		// Validate tenant ID to prevent SSRF
		if !validResourceID(tid) {
			writeError(w, http.StatusBadRequest, "invalid tenant ID format")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		// Build URL safely using url.Values for query parameters
		// tid is validated via validResourceID(), url.Values.Set() properly escapes all values
		u, _ := url.Parse(baseURL)
		u.Path = "/api/v1/intelligence/recommendations"
		q := u.Query()
		q.Set("tenant", tid)
		u.RawQuery = q.Encode()

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil) //#nosec G704
		copyHeaders(r, req)

		resp, err := http.DefaultClient.Do(req) //#nosec G704
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
		if os.Getenv("ENTERPRISE_LICENSE") == "" || os.Getenv("WADDLEAI_ENABLED") == "" {
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

		// Validate tenant ID to prevent SSRF
		if !validResourceID(tid) {
			writeError(w, http.StatusBadRequest, "invalid tenant ID format")
			return
		}

		// Validate node ID if provided
		if node != "" && !validResourceID(node) {
			writeError(w, http.StatusBadRequest, "invalid node ID format")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		// Build URL safely using url.Values for query parameters
		// tid and node are validated via validResourceID(), url.Values.Set() properly escapes all values
		u, _ := url.Parse(baseURL)
		u.Path = "/api/v1/predictive-drive/risk"
		q := u.Query()
		q.Set("tenant", tid)
		if node != "" {
			q.Set("node", node)
		}
		u.RawQuery = q.Encode()

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil) //#nosec G704
		copyHeaders(r, req)

		resp, err := http.DefaultClient.Do(req) //#nosec G704
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
		if os.Getenv("ENTERPRISE_LICENSE") == "" || os.Getenv("WADDLEAI_ENABLED") == "" {
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

		// Validate tenant ID to prevent SSRF
		if !validResourceID(tid) {
			writeError(w, http.StatusBadRequest, "invalid tenant ID format")
			return
		}

		// Validate severity if provided (alphanumeric only)
		if severity != "" && !validResourceID(severity) {
			writeError(w, http.StatusBadRequest, "invalid severity format")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		// Build URL safely using url.Values for query parameters
		// tid and severity are validated via validResourceID(), url.Values.Set() properly escapes all values
		u, _ := url.Parse(baseURL)
		u.Path = "/api/v1/anomaly/current"
		q := u.Query()
		q.Set("tenant", tid)
		if severity != "" {
			q.Set("severity", severity)
		}
		u.RawQuery = q.Encode()

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil) //#nosec G704
		copyHeaders(r, req)

		resp, err := http.DefaultClient.Do(req) //#nosec G704
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
