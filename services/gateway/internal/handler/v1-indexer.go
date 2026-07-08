package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"go.uber.org/zap"

	"github.com/penguintechinc/nest/services/gateway/internal/claims"
	"github.com/penguintechinc/nest/services/gateway/internal/config"
)

func indexerCatalogHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
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

		indexerURL := os.Getenv("INDEXER_URL")
		if indexerURL == "" {
			indexerURL = "http://nest-data-indexer:8090"
		}

		// Validate tenant ID to prevent SSRF
		if !validResourceID(tid) {
			writeError(w, http.StatusBadRequest, "invalid tenant ID format")
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Build URL safely using url.Values for query parameters
		// tid is validated via validResourceID(), url.Values.Set() properly escapes all values
		u, _ := url.Parse(indexerURL)
		u.Path = "/api/v1/indexer/catalog"
		q := u.Query()
		q.Set("tenant", tid)
		u.RawQuery = q.Encode()

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

func indexerScanHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
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

		indexerURL := os.Getenv("INDEXER_URL")
		if indexerURL == "" {
			indexerURL = "http://nest-data-indexer:8090"
		}

		body, _ := io.ReadAll(r.Body)
		var bodyMap map[string]interface{}
		json.Unmarshal(body, &bodyMap)
		bodyMap["tenant"] = tid
		newBody, _ := json.Marshal(bodyMap)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, indexerURL+"/api/v1/indexer/scan", bytes.NewReader(newBody)) //#nosec G704
		copyHeaders(r, req)
		req.Header.Set("Content-Type", "application/json")

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

func indexerPIITargetsHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if os.Getenv("ENTERPRISE_LICENSE") == "" {
			writeError(w, http.StatusPaymentRequired, "enterprise license required")
			return
		}

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

		indexerURL := os.Getenv("INDEXER_URL")
		if indexerURL == "" {
			indexerURL = "http://nest-data-indexer:8090"
		}

		// Validate tenant ID to prevent SSRF
		if !validResourceID(tid) {
			writeError(w, http.StatusBadRequest, "invalid tenant ID format")
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Build URL safely using url.Values for query parameters
		// tid is validated via validResourceID(), url.Values.Set() properly escapes all values
		u, _ := url.Parse(indexerURL)
		u.Path = "/api/v1/indexer/pii-targets"
		q := u.Query()
		q.Set("tenant", tid)
		u.RawQuery = q.Encode()

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

func policyEvaluateHandler(cfg config.Config, logger *zap.Logger) http.HandlerFunc {
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

		policyURL := os.Getenv("POLICY_ENGINE_URL")
		if policyURL == "" {
			policyURL = "http://nest-policy-engine:50058"
		}

		body, _ := io.ReadAll(r.Body)
		var bodyMap map[string]interface{}
		json.Unmarshal(body, &bodyMap)
		bodyMap["tenant"] = tid
		newBody, _ := json.Marshal(bodyMap)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, policyURL+"/api/v1/evaluate", bytes.NewReader(newBody)) //#nosec G704
		copyHeaders(r, req)
		req.Header.Set("Content-Type", "application/json")

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

func copyHeaders(src *http.Request, dst *http.Request) {
	for key, values := range src.Header {
		for _, v := range values {
			dst.Header.Add(key, v)
		}
	}
}
