package handler

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"go.uber.org/zap"

	"github.com/penguintechinc/nest/services/gateway/internal/claims"
	"github.com/penguintechinc/nest/services/gateway/internal/config"
)

func meteringHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
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

		costCalcURL := os.Getenv("COST_CALCULATOR_URL")
		if costCalcURL == "" {
			costCalcURL = "http://nest-cost-calculator:8091"
		}

		// Validate tenant ID to prevent SSRF
		if !validResourceID(tid) {
			writeError(w, http.StatusBadRequest, "invalid tenant ID format")
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Build URL safely using url.URL with url.PathEscape - tid is validated via validResourceID()
		u, _ := url.Parse(costCalcURL)
		u.Path = "/api/v1/billing/" + url.PathEscape(tid)

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil) //#nosec G704
		copyHeaders(r, req)

		resp, err := http.DefaultClient.Do(req) //#nosec G704
		if err != nil {
			writeError(w, http.StatusBadGateway, "upstream unavailable")
			return
		}
		defer resp.Body.Close()

		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}
}

func billingHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
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

		costCalcURL := os.Getenv("COST_CALCULATOR_URL")
		if costCalcURL == "" {
			costCalcURL = "http://nest-cost-calculator:8091"
		}

		// Validate tenant ID to prevent SSRF
		if !validResourceID(tid) {
			writeError(w, http.StatusBadRequest, "invalid tenant ID format")
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Build URL safely using url.URL with url.PathEscape - tid is validated via validResourceID()
		u, _ := url.Parse(costCalcURL)
		u.Path = "/api/v1/billing/" + url.PathEscape(tid) + "/summary"

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil) //#nosec G704
		copyHeaders(r, req)

		resp, err := http.DefaultClient.Do(req) //#nosec G704
		if err != nil {
			writeError(w, http.StatusBadGateway, "upstream unavailable")
			return
		}
		defer resp.Body.Close()

		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}
}
