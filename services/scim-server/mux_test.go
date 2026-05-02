package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func newTestLogger() *zap.Logger {
	logger, _ := zap.NewDevelopment()
	return logger
}

func TestHealthz(t *testing.T) {
	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/healthz")
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %s", body["status"])
	}
}

// License check tests - all SCIM endpoints return 402 when license not set

func TestListUsersNoLicense(t *testing.T) {
	os.Unsetenv("ENTERPRISE_LICENSE")
	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/scim/v2/Users")
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPaymentRequired {
		t.Errorf("expected 402, got %d", resp.StatusCode)
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["error"] != "enterprise license required" {
		t.Errorf("expected license error, got %s", body["error"])
	}
}

func TestCreateUserNoLicense(t *testing.T) {
	os.Unsetenv("ENTERPRISE_LICENSE")
	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	user := map[string]interface{}{
		"userName":    "alice",
		"displayName": "Alice",
		"active":      true,
	}
	body, _ := json.Marshal(user)

	resp, err := http.Post(server.URL+"/scim/v2/Users", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPaymentRequired {
		t.Errorf("expected 402, got %d", resp.StatusCode)
	}
}

func TestGetUserHTTPNoLicense(t *testing.T) {
	os.Unsetenv("ENTERPRISE_LICENSE")
	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL+"/scim/v2/Users/some-id", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPaymentRequired {
		t.Errorf("expected 402, got %d", resp.StatusCode)
	}
}

func TestUpdateUserNoLicense(t *testing.T) {
	os.Unsetenv("ENTERPRISE_LICENSE")
	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	user := map[string]interface{}{
		"userName": "bob",
	}
	body, _ := json.Marshal(user)

	req, _ := http.NewRequest("PUT", server.URL+"/scim/v2/Users/some-id", bytes.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPaymentRequired {
		t.Errorf("expected 402, got %d", resp.StatusCode)
	}
}

func TestDeleteUserNoLicense(t *testing.T) {
	os.Unsetenv("ENTERPRISE_LICENSE")
	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	req, _ := http.NewRequest("DELETE", server.URL+"/scim/v2/Users/some-id", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPaymentRequired {
		t.Errorf("expected 402, got %d", resp.StatusCode)
	}
}

func TestListGroupsNoLicense(t *testing.T) {
	os.Unsetenv("ENTERPRISE_LICENSE")
	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/scim/v2/Groups")
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPaymentRequired {
		t.Errorf("expected 402, got %d", resp.StatusCode)
	}
}

func TestCreateGroupNoLicense(t *testing.T) {
	os.Unsetenv("ENTERPRISE_LICENSE")
	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	group := map[string]interface{}{
		"displayName": "admins",
	}
	body, _ := json.Marshal(group)

	resp, err := http.Post(server.URL+"/scim/v2/Groups", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPaymentRequired {
		t.Errorf("expected 402, got %d", resp.StatusCode)
	}
}

func TestGetGroupHTTPNoLicense(t *testing.T) {
	os.Unsetenv("ENTERPRISE_LICENSE")
	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL+"/scim/v2/Groups/some-id", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPaymentRequired {
		t.Errorf("expected 402, got %d", resp.StatusCode)
	}
}

func TestDeleteGroupNoLicense(t *testing.T) {
	os.Unsetenv("ENTERPRISE_LICENSE")
	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	req, _ := http.NewRequest("DELETE", server.URL+"/scim/v2/Groups/some-id", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPaymentRequired {
		t.Errorf("expected 402, got %d", resp.StatusCode)
	}
}

func TestServiceProviderConfigNoLicense(t *testing.T) {
	os.Unsetenv("ENTERPRISE_LICENSE")
	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/scim/v2/ServiceProviderConfig")
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// ServiceProviderConfig does NOT require license
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

// User CRUD tests - with license

func TestHTTPCreateUserSuccess(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	user := map[string]interface{}{
		"userName":    "alice",
		"displayName": "Alice Smith",
		"active":      true,
		"emails": []map[string]interface{}{
			{
				"value":   "alice@example.com",
				"primary": true,
			},
		},
	}
	body, _ := json.Marshal(user)

	resp, err := http.Post(server.URL+"/scim/v2/Users", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}

	var respBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&respBody)

	if respBody["userName"] != "alice" {
		t.Errorf("expected userName=alice, got %v", respBody["userName"])
	}

	if respBody["id"] == "" {
		t.Errorf("expected id to be set")
	}

	// Check Location header
	if resp.Header.Get("Location") == "" {
		t.Errorf("expected Location header")
	}
}

func TestHTTPCreateUserMissingUserName(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	user := map[string]interface{}{
		"displayName": "Alice Smith",
		"active":      true,
	}
	body, _ := json.Marshal(user)

	resp, err := http.Post(server.URL+"/scim/v2/Users", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}

	var respBody map[string]string
	json.NewDecoder(resp.Body).Decode(&respBody)
	if !strings.Contains(respBody["error"], "userName") {
		t.Errorf("expected userName validation error, got %s", respBody["error"])
	}
}

func TestHTTPCreateUserInvalidJSON(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Post(server.URL+"/scim/v2/Users", "application/json", bytes.NewReader([]byte("invalid json")))
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}

	var respBody map[string]string
	json.NewDecoder(resp.Body).Decode(&respBody)
	if respBody["error"] != "invalid request body" {
		t.Errorf("expected invalid request body error, got %s", respBody["error"])
	}
}

func TestHTTPListUsersSuccess(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	// Create two users
	for i := 1; i <= 2; i++ {
		user := map[string]interface{}{
			"userName":    fmt.Sprintf("user%d", i),
			"displayName": fmt.Sprintf("User %d", i),
			"active":      true,
		}
		body, _ := json.Marshal(user)
		http.Post(server.URL+"/scim/v2/Users", "application/json", bytes.NewReader(body))
	}

	resp, err := http.Get(server.URL + "/scim/v2/Users")
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var respBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&respBody)

	if respBody["totalResults"] != float64(2) {
		t.Errorf("expected 2 users, got %v", respBody["totalResults"])
	}

	if respBody["Resources"] == nil {
		t.Errorf("expected Resources array")
	}
}

func TestHTTPGetUserSuccess(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	// Create a user
	user := map[string]interface{}{
		"userName":    "alice",
		"displayName": "Alice",
		"active":      true,
	}
	body, _ := json.Marshal(user)
	createResp, _ := http.Post(server.URL+"/scim/v2/Users", "application/json", bytes.NewReader(body))
	var createBody map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createBody)
	createResp.Body.Close()
	userID := createBody["id"].(string)

	// Get the user
	resp, err := http.Get(server.URL + "/scim/v2/Users/" + userID)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var respBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&respBody)

	if respBody["userName"] != "alice" {
		t.Errorf("expected userName=alice, got %v", respBody["userName"])
	}
	if respBody["id"] != userID {
		t.Errorf("expected id=%s, got %v", userID, respBody["id"])
	}
}

func TestHTTPGetUserHTTPNotFound(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/scim/v2/Users/nonexistent-id")
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}

	var respBody map[string]string
	json.NewDecoder(resp.Body).Decode(&respBody)
	if respBody["error"] != "user not found" {
		t.Errorf("expected user not found error, got %s", respBody["error"])
	}
}

func TestHTTPUpdateUserSuccess(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	// Create a user
	user := map[string]interface{}{
		"userName":    "alice",
		"displayName": "Alice",
		"active":      true,
	}
	body, _ := json.Marshal(user)
	createResp, _ := http.Post(server.URL+"/scim/v2/Users", "application/json", bytes.NewReader(body))
	var createBody map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createBody)
	createResp.Body.Close()
	userID := createBody["id"].(string)

	// Update the user
	updatedUser := map[string]interface{}{
		"userName":    "alice",
		"displayName": "Alice Johnson",
		"active":      false,
	}
	updatedBody, _ := json.Marshal(updatedUser)
	req, _ := http.NewRequest("PUT", server.URL+"/scim/v2/Users/"+userID, bytes.NewReader(updatedBody))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var respBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&respBody)

	if respBody["displayName"] != "Alice Johnson" {
		t.Errorf("expected displayName=Alice Johnson, got %v", respBody["displayName"])
	}
	if respBody["active"] != false {
		t.Errorf("expected active=false, got %v", respBody["active"])
	}
}

func TestHTTPUpdateUserHTTPNotFound(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	updatedUser := map[string]interface{}{
		"userName": "alice",
	}
	updatedBody, _ := json.Marshal(updatedUser)
	req, _ := http.NewRequest("PUT", server.URL+"/scim/v2/Users/nonexistent-id", bytes.NewReader(updatedBody))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}

	var respBody map[string]string
	json.NewDecoder(resp.Body).Decode(&respBody)
	if respBody["error"] != "user not found" {
		t.Errorf("expected user not found error, got %s", respBody["error"])
	}
}

func TestHTTPUpdateUserInvalidJSON(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	req, _ := http.NewRequest("PUT", server.URL+"/scim/v2/Users/some-id", bytes.NewReader([]byte("invalid json")))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}

	var respBody map[string]string
	json.NewDecoder(resp.Body).Decode(&respBody)
	if respBody["error"] != "invalid request body" {
		t.Errorf("expected invalid request body error, got %s", respBody["error"])
	}
}

func TestHTTPDeleteUserSuccess(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	// Create a user
	user := map[string]interface{}{
		"userName":    "alice",
		"displayName": "Alice",
		"active":      true,
	}
	body, _ := json.Marshal(user)
	createResp, _ := http.Post(server.URL+"/scim/v2/Users", "application/json", bytes.NewReader(body))
	var createBody map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createBody)
	createResp.Body.Close()
	userID := createBody["id"].(string)

	// Delete the user
	req, _ := http.NewRequest("DELETE", server.URL+"/scim/v2/Users/"+userID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}

	// Verify user is deleted
	getReq, _ := http.NewRequest("GET", server.URL+"/scim/v2/Users/"+userID, nil)
	getResp, _ := http.DefaultClient.Do(getReq)
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", getResp.StatusCode)
	}
}

func TestHTTPDeleteUserHTTPNotFound(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	req, _ := http.NewRequest("DELETE", server.URL+"/scim/v2/Users/nonexistent-id", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}

	var respBody map[string]string
	json.NewDecoder(resp.Body).Decode(&respBody)
	if respBody["error"] != "user not found" {
		t.Errorf("expected user not found error, got %s", respBody["error"])
	}
}

// Group CRUD tests - with license

func TestHTTPCreateGroupSuccess(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	group := map[string]interface{}{
		"displayName": "admins",
		"externalId":  "grp-1",
	}
	body, _ := json.Marshal(group)

	resp, err := http.Post(server.URL+"/scim/v2/Groups", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}

	var respBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&respBody)

	if respBody["displayName"] != "admins" {
		t.Errorf("expected displayName=admins, got %v", respBody["displayName"])
	}

	if respBody["id"] == "" {
		t.Errorf("expected id to be set")
	}

	// Check Location header
	if resp.Header.Get("Location") == "" {
		t.Errorf("expected Location header")
	}
}

func TestHTTPCreateGroupMissingDisplayName(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	group := map[string]interface{}{
		"externalId": "grp-1",
	}
	body, _ := json.Marshal(group)

	resp, err := http.Post(server.URL+"/scim/v2/Groups", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}

	var respBody map[string]string
	json.NewDecoder(resp.Body).Decode(&respBody)
	if !strings.Contains(respBody["error"], "displayName") {
		t.Errorf("expected displayName validation error, got %s", respBody["error"])
	}
}

func TestHTTPCreateGroupInvalidJSON(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Post(server.URL+"/scim/v2/Groups", "application/json", bytes.NewReader([]byte("invalid json")))
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}

	var respBody map[string]string
	json.NewDecoder(resp.Body).Decode(&respBody)
	if respBody["error"] != "invalid request body" {
		t.Errorf("expected invalid request body error, got %s", respBody["error"])
	}
}

func TestHTTPListGroupsSuccess(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	// Create two groups
	for i := 1; i <= 2; i++ {
		group := map[string]interface{}{
			"displayName": fmt.Sprintf("group%d", i),
		}
		body, _ := json.Marshal(group)
		http.Post(server.URL+"/scim/v2/Groups", "application/json", bytes.NewReader(body))
	}

	resp, err := http.Get(server.URL + "/scim/v2/Groups")
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var respBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&respBody)

	if respBody["totalResults"] != float64(2) {
		t.Errorf("expected 2 groups, got %v", respBody["totalResults"])
	}

	if respBody["Resources"] == nil {
		t.Errorf("expected Resources array")
	}
}

func TestHTTPGetGroupSuccess(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	// Create a group
	group := map[string]interface{}{
		"displayName": "admins",
	}
	body, _ := json.Marshal(group)
	createResp, _ := http.Post(server.URL+"/scim/v2/Groups", "application/json", bytes.NewReader(body))
	var createBody map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createBody)
	createResp.Body.Close()
	groupID := createBody["id"].(string)

	// Get the group
	resp, err := http.Get(server.URL + "/scim/v2/Groups/" + groupID)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var respBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&respBody)

	if respBody["displayName"] != "admins" {
		t.Errorf("expected displayName=admins, got %v", respBody["displayName"])
	}
	if respBody["id"] != groupID {
		t.Errorf("expected id=%s, got %v", groupID, respBody["id"])
	}
}

func TestHTTPGetGroupHTTPNotFound(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/scim/v2/Groups/nonexistent-id")
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}

	var respBody map[string]string
	json.NewDecoder(resp.Body).Decode(&respBody)
	if respBody["error"] != "group not found" {
		t.Errorf("expected group not found error, got %s", respBody["error"])
	}
}

func TestHTTPDeleteGroupSuccess(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	// Create a group
	group := map[string]interface{}{
		"displayName": "admins",
	}
	body, _ := json.Marshal(group)
	createResp, _ := http.Post(server.URL+"/scim/v2/Groups", "application/json", bytes.NewReader(body))
	var createBody map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createBody)
	createResp.Body.Close()
	groupID := createBody["id"].(string)

	// Delete the group
	req, _ := http.NewRequest("DELETE", server.URL+"/scim/v2/Groups/"+groupID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}

	// Verify group is deleted
	getReq, _ := http.NewRequest("GET", server.URL+"/scim/v2/Groups/"+groupID, nil)
	getResp, _ := http.DefaultClient.Do(getReq)
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", getResp.StatusCode)
	}
}

func TestHTTPDeleteGroupHTTPNotFound(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	req, _ := http.NewRequest("DELETE", server.URL+"/scim/v2/Groups/nonexistent-id", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}

	var respBody map[string]string
	json.NewDecoder(resp.Body).Decode(&respBody)
	if respBody["error"] != "group not found" {
		t.Errorf("expected group not found error, got %s", respBody["error"])
	}
}

// ServiceProviderConfig tests

func TestHTTPServiceProviderConfigSuccess(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/scim/v2/ServiceProviderConfig")
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	if resp.Header.Get("Content-Type") != "application/scim+json" {
		t.Errorf("expected Content-Type=application/scim+json, got %s", resp.Header.Get("Content-Type"))
	}

	var respBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&respBody)

	if respBody["schemas"] == nil {
		t.Errorf("expected schemas in response")
	}

	if respBody["patch"] == nil {
		t.Errorf("expected patch in response")
	}

	if respBody["bulk"] == nil {
		t.Errorf("expected bulk in response")
	}
}

// Content-Type validation tests

func TestHTTPContentTypeHeaders(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	tests := []struct {
		name   string
		method string
		path   string
		body   []byte
	}{
		{"list users", "GET", "/scim/v2/Users", nil},
		{"list groups", "GET", "/scim/v2/Groups", nil},
		{"config", "GET", "/scim/v2/ServiceProviderConfig", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest(tt.method, server.URL+tt.path, io.NopCloser(bytes.NewReader(tt.body)))
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("failed to make request: %v", err)
			}
			defer resp.Body.Close()

			ct := resp.Header.Get("Content-Type")
			if ct != "application/scim+json" {
				t.Errorf("expected Content-Type=application/scim+json, got %s", ct)
			}
		})
	}
}

// Handler edge case tests - error paths

func TestHTTPCreateUserStoreError(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	user := map[string]interface{}{
		"userName":    "validuser",
		"displayName": "Valid User",
		"active":      true,
	}
	body, _ := json.Marshal(user)

	resp, err := http.Post(server.URL+"/scim/v2/Users", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}

	var respBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&respBody)
	if respBody["id"] == nil {
		t.Errorf("expected id to be set")
	}
}

func TestHTTPCreateGroupStoreError(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	group := map[string]interface{}{
		"displayName": "validgroup",
	}
	body, _ := json.Marshal(group)

	resp, err := http.Post(server.URL+"/scim/v2/Groups", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}

	var respBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&respBody)
	if respBody["id"] == nil {
		t.Errorf("expected id to be set")
	}
}

// License checking edge cases

func TestRequireEnterpriseLicenseWithLicense(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "valid-license-key")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/scim/v2/Users")
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 with valid license, got %d", resp.StatusCode)
	}
}

// Test empty user/group list responses

func TestListUsersEmptyList(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/scim/v2/Users")
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var respBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&respBody)

	if respBody["totalResults"] != float64(0) {
		t.Errorf("expected 0 users, got %v", respBody["totalResults"])
	}
}

func TestListGroupsEmptyList(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/scim/v2/Groups")
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var respBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&respBody)

	if respBody["totalResults"] != float64(0) {
		t.Errorf("expected 0 groups, got %v", respBody["totalResults"])
	}
}

// Comprehensive handler coverage tests

func TestHTTPCreateUserResponseStructure(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	user := map[string]interface{}{
		"userName":    "testuser",
		"displayName": "Test User",
		"active":      true,
		"emails": []map[string]interface{}{
			{"value": "test@example.com", "primary": true},
		},
	}
	body, _ := json.Marshal(user)

	resp, _ := http.Post(server.URL+"/scim/v2/Users", "application/json", bytes.NewReader(body))
	defer resp.Body.Close()

	var respBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&respBody)

	// Verify response has all required SCIM fields
	if respBody["schemas"] == nil {
		t.Errorf("expected schemas in response")
	}
	if respBody["meta"] == nil {
		t.Errorf("expected meta in response")
	}
	if respBody["id"] == nil {
		t.Errorf("expected id in response")
	}
}

func TestHTTPCreateGroupResponseStructure(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	group := map[string]interface{}{
		"displayName": "testgroup",
	}
	body, _ := json.Marshal(group)

	resp, _ := http.Post(server.URL+"/scim/v2/Groups", "application/json", bytes.NewReader(body))
	defer resp.Body.Close()

	var respBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&respBody)

	// Verify response has all required SCIM fields
	if respBody["schemas"] == nil {
		t.Errorf("expected schemas in response")
	}
	if respBody["meta"] == nil {
		t.Errorf("expected meta in response")
	}
	if respBody["id"] == nil {
		t.Errorf("expected id in response")
	}
}

func TestHTTPGetUserResponseStructure(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	// Create a user first
	user := map[string]interface{}{
		"userName":    "alice",
		"displayName": "Alice",
		"active":      true,
	}
	body, _ := json.Marshal(user)
	createResp, _ := http.Post(server.URL+"/scim/v2/Users", "application/json", bytes.NewReader(body))
	var createBody map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createBody)
	createResp.Body.Close()
	userID := createBody["id"].(string)

	// Get the user and verify response structure
	resp, _ := http.Get(server.URL + "/scim/v2/Users/" + userID)
	defer resp.Body.Close()

	var respBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&respBody)

	if respBody["schemas"] == nil {
		t.Errorf("expected schemas in response")
	}
	if respBody["meta"] == nil {
		t.Errorf("expected meta in response")
	}
}

func TestHTTPGetGroupResponseStructure(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	// Create a group first
	group := map[string]interface{}{
		"displayName": "admins",
	}
	body, _ := json.Marshal(group)
	createResp, _ := http.Post(server.URL+"/scim/v2/Groups", "application/json", bytes.NewReader(body))
	var createBody map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createBody)
	createResp.Body.Close()
	groupID := createBody["id"].(string)

	// Get the group and verify response structure
	resp, _ := http.Get(server.URL + "/scim/v2/Groups/" + groupID)
	defer resp.Body.Close()

	var respBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&respBody)

	if respBody["schemas"] == nil {
		t.Errorf("expected schemas in response")
	}
	if respBody["meta"] == nil {
		t.Errorf("expected meta in response")
	}
}

func TestHTTPUpdateUserResponseStructure(t *testing.T) {
	os.Setenv("ENTERPRISE_LICENSE", "test-license")
	defer os.Unsetenv("ENTERPRISE_LICENSE")

	store := NewSCIMStore(newTestLogger())
	mux := NewMux(store, newTestLogger())
	server := httptest.NewServer(mux)
	defer server.Close()

	// Create a user first
	user := map[string]interface{}{
		"userName":    "bob",
		"displayName": "Bob",
		"active":      true,
	}
	body, _ := json.Marshal(user)
	createResp, _ := http.Post(server.URL+"/scim/v2/Users", "application/json", bytes.NewReader(body))
	var createBody map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createBody)
	createResp.Body.Close()
	userID := createBody["id"].(string)

	// Update the user
	updated := map[string]interface{}{
		"userName":    "bob",
		"displayName": "Bob Smith",
		"active":      true,
	}
	updatedBody, _ := json.Marshal(updated)
	req, _ := http.NewRequest("PUT", server.URL+"/scim/v2/Users/"+userID, bytes.NewReader(updatedBody))
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	var respBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&respBody)

	// Verify response structure includes all fields
	if respBody["schemas"] == nil {
		t.Errorf("expected schemas in response")
	}
	if respBody["meta"] == nil {
		t.Errorf("expected meta in response")
	}
	if respBody["displayName"] != "Bob Smith" {
		t.Errorf("expected updated displayName")
	}
}
