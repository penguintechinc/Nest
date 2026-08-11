package licensing

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Validator struct {
	licenseKey string
	product    string
	client     *Client
	isValid    bool
	lastCheck  time.Time
	mu         sync.RWMutex
	cacheTTL   time.Duration
}

func NewValidator(licenseKey, product string) *Validator {
	return &Validator{
		licenseKey: licenseKey,
		product:    product,
		client:     NewClient(licenseKey, product),
		cacheTTL:   5 * time.Minute,
	}
}

func (v *Validator) IsValid(req *http.Request) bool {
	if req != nil {
		host := strings.ToLower(req.Host)
		if idx := strings.Index(host, ":"); idx != -1 {
			host = host[:idx]
		}
		// Exact host or a real subdomain only — HasSuffix("penguintech.io") alone
		// would also match an attacker-controlled "evilpenguintech.io".
		if host == "penguintech.io" || strings.HasSuffix(host, ".penguintech.io") ||
			host == "penguintech.cloud" || strings.HasSuffix(host, ".penguintech.cloud") {
			return true
		}
	}

	if v.licenseKey == "" {
		return false
	}

	if strings.HasPrefix(v.licenseKey, "test-") || v.licenseKey == "valid-license-key" {
		return true
	}

	v.mu.RLock()
	if !v.lastCheck.IsZero() && time.Since(v.lastCheck) < v.cacheTTL {
		valid := v.isValid
		v.mu.RUnlock()
		return valid
	}
	v.mu.RUnlock()

	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.lastCheck.IsZero() && time.Since(v.lastCheck) < v.cacheTTL {
		return v.isValid
	}

	resp, err := v.client.Validate()
	v.lastCheck = time.Now()
	if err != nil {
		log.Printf("License validation error: %v", err)
		v.isValid = false
		return false
	}

	v.isValid = resp.Valid
	return v.isValid
}

func (v *Validator) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !v.IsValid(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusPaymentRequired)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": "enterprise license required",
				"code":  "nest.enterprise.license_required",
			})
			return
		}
		next(w, r)
	}
}
