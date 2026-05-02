package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestSAMLEnabled(t *testing.T) {
	tests := []struct {
		name              string
		enterpriseLicense string
		samlIDPMetadata   string
		expected          bool
	}{
		{
			name:              "both set",
			enterpriseLicense: "license-key",
			samlIDPMetadata:   "http://idp/metadata",
			expected:          true,
		},
		{
			name:              "license not set",
			enterpriseLicense: "",
			samlIDPMetadata:   "http://idp/metadata",
			expected:          false,
		},
		{
			name:              "idp metadata not set",
			enterpriseLicense: "license-key",
			samlIDPMetadata:   "",
			expected:          false,
		},
		{
			name:              "neither set",
			enterpriseLicense: "",
			samlIDPMetadata:   "",
			expected:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original env vars
			origLicense := os.Getenv("ENTERPRISE_LICENSE")
			origMetadata := os.Getenv("SAML_IDP_METADATA_URL")
			defer func() {
				if origLicense != "" {
					os.Setenv("ENTERPRISE_LICENSE", origLicense)
				} else {
					os.Unsetenv("ENTERPRISE_LICENSE")
				}
				if origMetadata != "" {
					os.Setenv("SAML_IDP_METADATA_URL", origMetadata)
				} else {
					os.Unsetenv("SAML_IDP_METADATA_URL")
				}
			}()

			// Set test env vars
			if tt.enterpriseLicense != "" {
				os.Setenv("ENTERPRISE_LICENSE", tt.enterpriseLicense)
			} else {
				os.Unsetenv("ENTERPRISE_LICENSE")
			}
			if tt.samlIDPMetadata != "" {
				os.Setenv("SAML_IDP_METADATA_URL", tt.samlIDPMetadata)
			} else {
				os.Unsetenv("SAML_IDP_METADATA_URL")
			}

			got := SAMLEnabled()
			if got != tt.expected {
				t.Errorf("SAMLEnabled() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSAMLACSHandler_Disabled(t *testing.T) {
	// Save and clear env vars to simulate SAML disabled
	origLicense := os.Getenv("ENTERPRISE_LICENSE")
	origMetadata := os.Getenv("SAML_IDP_METADATA_URL")
	defer func() {
		if origLicense != "" {
			os.Setenv("ENTERPRISE_LICENSE", origLicense)
		} else {
			os.Unsetenv("ENTERPRISE_LICENSE")
		}
		if origMetadata != "" {
			os.Setenv("SAML_IDP_METADATA_URL", origMetadata)
		} else {
			os.Unsetenv("SAML_IDP_METADATA_URL")
		}
	}()

	os.Unsetenv("ENTERPRISE_LICENSE")
	os.Unsetenv("SAML_IDP_METADATA_URL")

	req := httptest.NewRequest("POST", "/saml/acs", bytes.NewReader([]byte("saml_response=...")))
	w := httptest.NewRecorder()

	SAMLACSHandler(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("SAMLACSHandler status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] != "saml not configured" {
		t.Errorf("error message = %q, want 'saml not configured'", resp["error"])
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}
}

func TestSAMLACSHandler_Enabled(t *testing.T) {
	// Save and set env vars to simulate SAML enabled
	origLicense := os.Getenv("ENTERPRISE_LICENSE")
	origMetadata := os.Getenv("SAML_IDP_METADATA_URL")
	defer func() {
		if origLicense != "" {
			os.Setenv("ENTERPRISE_LICENSE", origLicense)
		} else {
			os.Unsetenv("ENTERPRISE_LICENSE")
		}
		if origMetadata != "" {
			os.Setenv("SAML_IDP_METADATA_URL", origMetadata)
		} else {
			os.Unsetenv("SAML_IDP_METADATA_URL")
		}
	}()

	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	os.Setenv("SAML_IDP_METADATA_URL", "http://idp/metadata")

	req := httptest.NewRequest("POST", "/saml/acs", bytes.NewReader([]byte("saml_response=...")))
	w := httptest.NewRecorder()

	SAMLACSHandler(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("SAMLACSHandler status = %d, want %d", w.Code, http.StatusNotImplemented)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] != "saml acs not yet implemented" {
		t.Errorf("error message = %q, want 'saml acs not yet implemented'", resp["error"])
	}
}

func TestSAMLMetadataHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/saml/metadata", nil)
	w := httptest.NewRecorder()

	SAMLMetadataHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("SAMLMetadataHandler status = %d, want %d", w.Code, http.StatusOK)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/xml" {
		t.Errorf("Content-Type = %q, want application/xml", contentType)
	}

	body := w.Body.String()
	if !bytes.Contains([]byte(body), []byte("<?xml")) {
		t.Error("response should be XML")
	}
	if !bytes.Contains([]byte(body), []byte("EntityDescriptor")) {
		t.Error("response should contain EntityDescriptor")
	}
	if !bytes.Contains([]byte(body), []byte("nest-sp")) {
		t.Error("response should contain nest-sp entity ID")
	}
}

func TestSAMLMetadataHandler_CorrectStructure(t *testing.T) {
	req := httptest.NewRequest("GET", "/saml/metadata", nil)
	w := httptest.NewRecorder()

	SAMLMetadataHandler(w, req)

	body := w.Body.String()
	expectedXML := `<?xml version="1.0"?><EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="nest-sp"></EntityDescriptor>`

	if body != expectedXML {
		t.Errorf("SAMLMetadataHandler response = %q, want %q", body, expectedXML)
	}
}
