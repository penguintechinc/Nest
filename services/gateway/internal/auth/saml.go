package auth

import (
	"encoding/json"
	"net/http"
	"os"
)

// SAMLEnabled returns true if SAML is configured (enterprise-licensed + SAML_IDP_METADATA_URL set).
func SAMLEnabled() bool {
	return os.Getenv("ENTERPRISE_LICENSE") != "" && os.Getenv("SAML_IDP_METADATA_URL") != ""
}

// SAMLACSHandler handles the SAML Assertion Consumer Service endpoint.
// Returns 501 Not Implemented in this stub; real implementation uses a SAML library.
func SAMLACSHandler(w http.ResponseWriter, r *http.Request) {
	if !SAMLEnabled() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "saml not configured",
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(map[string]string{
		"error": "saml acs not yet implemented",
	})
}

// SAMLMetadataHandler serves the SP metadata XML.
func SAMLMetadataHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`<?xml version="1.0"?><EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="nest-sp"></EntityDescriptor>`))
}
