package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

func TestSagaEngineRoutes(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSagaStore(logger)
	srv := httptest.NewServer(NewMux(store, logger))
	defer srv.Close()

	t.Run("GET /healthz", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/healthz")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/templates - create template", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"id":   "tpl-1",
			"name": "Test Template",
			"steps": []interface{}{
				map[string]interface{}{"name": "step1", "action": "provision"},
			},
		})
		resp, err := http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("expected 201, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/templates - invalid request", func(t *testing.T) {
		resp, err := http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader([]byte("invalid")))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/templates - list templates", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/templates")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/templates/{id} - get template", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"name": "Template 2",
			"steps": []interface{}{
				map[string]interface{}{"name": "step1", "action": "provision"},
			},
		})
		cr, err := http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer cr.Body.Close()
		var tmpl map[string]interface{}
		json.NewDecoder(cr.Body).Decode(&tmpl)
		id := tmpl["id"].(string)

		resp, err := http.Get(srv.URL + "/api/v1/templates/" + id)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/templates/{id} - not found", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/templates/nonexistent")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/workflows - start workflow", func(t *testing.T) {
		// Create a template and extract its auto-generated ID
		body, _ := json.Marshal(map[string]interface{}{
			"name": "Template 3",
			"steps": []interface{}{
				map[string]interface{}{"name": "step1", "action": "provision"},
			},
		})
		cr, err := http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer cr.Body.Close()
		var tmpl3 map[string]interface{}
		json.NewDecoder(cr.Body).Decode(&tmpl3)
		tplID := tmpl3["id"].(string)

		// Start workflow using the real template ID
		workflowBody, _ := json.Marshal(map[string]interface{}{
			"templateId": tplID,
			"tenant":     "test-tenant",
		})
		resp, err := http.Post(srv.URL+"/api/v1/workflows", "application/json", bytes.NewReader(workflowBody))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("expected 202, got %d", resp.StatusCode)
		}
		if resp.Header.Get("Location") == "" {
			t.Error("expected Location header")
		}
	})

	t.Run("POST /api/v1/workflows - missing required fields", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"templateId": "tpl-4",
		})
		resp, err := http.Post(srv.URL+"/api/v1/workflows", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/workflows - list workflows", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/workflows")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/workflows - filter by tenant", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/workflows?tenant=test-tenant")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/workflows/{id} - get workflow", func(t *testing.T) {
		// Create template and start workflow
		body, _ := json.Marshal(map[string]interface{}{
			"id":    "tpl-5",
			"name":  "Template 5",
			"steps": []interface{}{},
		})
		http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))

		workflowBody, _ := json.Marshal(map[string]interface{}{
			"templateId": "tpl-5",
			"tenant":     "test-tenant",
		})
		resp, err := http.Post(srv.URL+"/api/v1/workflows", "application/json", bytes.NewReader(workflowBody))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		var workflow map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&workflow)
		resp.Body.Close()

		if workflowID, ok := workflow["id"].(string); ok {
			resp, err := http.Get(srv.URL + "/api/v1/workflows/" + workflowID)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("expected 200, got %d", resp.StatusCode)
			}
		}
	})

	t.Run("GET /api/v1/workflows/{id} - not found", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/workflows/nonexistent")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/workflows/{id}/cancel - cancel workflow", func(t *testing.T) {
		// Create template and start workflow
		body, _ := json.Marshal(map[string]interface{}{
			"id":    "tpl-6",
			"name":  "Template 6",
			"steps": []interface{}{},
		})
		http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))

		workflowBody, _ := json.Marshal(map[string]interface{}{
			"templateId": "tpl-6",
			"tenant":     "test-tenant",
		})
		resp, err := http.Post(srv.URL+"/api/v1/workflows", "application/json", bytes.NewReader(workflowBody))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		var workflow map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&workflow)
		resp.Body.Close()

		if workflowID, ok := workflow["id"].(string); ok {
			req, _ := http.NewRequest("POST", srv.URL+"/api/v1/workflows/"+workflowID+"/cancel", nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("expected 200, got %d", resp.StatusCode)
			}
		}
	})

	t.Run("POST /api/v1/workflows/{id}/retry - retry workflow", func(t *testing.T) {
		// Create template and start workflow
		body, _ := json.Marshal(map[string]interface{}{
			"id":    "tpl-7",
			"name":  "Template 7",
			"steps": []interface{}{},
		})
		http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))

		workflowBody, _ := json.Marshal(map[string]interface{}{
			"templateId": "tpl-7",
			"tenant":     "test-tenant",
		})
		resp, err := http.Post(srv.URL+"/api/v1/workflows", "application/json", bytes.NewReader(workflowBody))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		var workflow map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&workflow)
		resp.Body.Close()

		if workflowID, ok := workflow["id"].(string); ok {
			req, _ := http.NewRequest("POST", srv.URL+"/api/v1/workflows/"+workflowID+"/retry", nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()
			// Retry should succeed or fail with 400 depending on workflow state
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadRequest {
				t.Errorf("expected 200 or 400, got %d", resp.StatusCode)
			}
		}
	})

	t.Run("POST /api/v1/workflows/{id}/retry - workflow not found", func(t *testing.T) {
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/workflows/nonexistent/retry", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/workflows/{id}/cancel - workflow not found", func(t *testing.T) {
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/workflows/nonexistent/cancel", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/workflows - invalid JSON", func(t *testing.T) {
		resp, err := http.Post(srv.URL+"/api/v1/workflows", "application/json", bytes.NewReader([]byte("invalid")))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/workflows - empty templateId", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"templateId": "",
			"tenant":     "test-tenant",
		})
		resp, err := http.Post(srv.URL+"/api/v1/workflows", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/templates - create with no steps", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"id":    "tpl-empty",
			"name":  "Empty Template",
			"steps": []interface{}{},
		})
		resp, err := http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/workflows - start with nonexistent template", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"templateId": "nonexistent-template",
			"tenant":     "test-tenant",
		})
		resp, err := http.Post(srv.URL+"/api/v1/workflows", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/templates - verify content-type", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/templates")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if contentType := resp.Header.Get("Content-Type"); contentType != "application/json" {
			t.Errorf("expected application/json, got %s", contentType)
		}
	})

	t.Run("GET /healthz - verify status response", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/healthz")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		var result map[string]string
		json.NewDecoder(resp.Body).Decode(&result)
		if result["status"] != "ok" {
			t.Errorf("expected status ok, got %v", result["status"])
		}
	})

	t.Run("POST /api/v1/templates - verify created response", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"name": "Template Check",
			"steps": []interface{}{
				map[string]interface{}{"name": "step1", "action": "test"},
			},
		})
		resp, err := http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		if result["id"] == "" {
			t.Error("expected template ID in response")
		}
	})

	t.Run("POST /api/v1/workflows/{id}/retry - retry requires failed status", func(t *testing.T) {
		// Create template and start workflow
		body, _ := json.Marshal(map[string]interface{}{
			"name": "Template Retry Test",
			"steps": []interface{}{
				map[string]interface{}{"name": "step1", "action": "test"},
			},
		})
		cr, err := http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		var tmpl map[string]interface{}
		json.NewDecoder(cr.Body).Decode(&tmpl)
		cr.Body.Close()
		tplID := tmpl["id"].(string)

		workflowBody, _ := json.Marshal(map[string]interface{}{
			"templateId": tplID,
			"tenant":     "test-tenant",
		})
		resp, err := http.Post(srv.URL+"/api/v1/workflows", "application/json", bytes.NewReader(workflowBody))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		var workflow map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&workflow)
		resp.Body.Close()
		workflowID := workflow["id"].(string)

		// First cancel it to set it to failed state
		cancelReq, _ := http.NewRequest("POST", srv.URL+"/api/v1/workflows/"+workflowID+"/cancel", nil)
		http.DefaultClient.Do(cancelReq)

		// Now retry should work
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/workflows/"+workflowID+"/retry", nil)
		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/workflows/{id}/cancel - verify response", func(t *testing.T) {
		// Create template and start workflow
		body, _ := json.Marshal(map[string]interface{}{
			"name": "Template Cancel Check",
			"steps": []interface{}{
				map[string]interface{}{"name": "step1", "action": "test"},
			},
		})
		cr, err := http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		var tmpl map[string]interface{}
		json.NewDecoder(cr.Body).Decode(&tmpl)
		cr.Body.Close()
		tplID := tmpl["id"].(string)

		workflowBody, _ := json.Marshal(map[string]interface{}{
			"templateId": tplID,
			"tenant":     "test-tenant",
		})
		resp, err := http.Post(srv.URL+"/api/v1/workflows", "application/json", bytes.NewReader(workflowBody))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		var workflow map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&workflow)
		resp.Body.Close()
		workflowID := workflow["id"].(string)

		// Cancel immediately
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/workflows/"+workflowID+"/cancel", nil)
		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		if result["status"] != "failed" {
			t.Errorf("expected status failed, got %v", result["status"])
		}
	})

	t.Run("POST /api/v1/workflows/{id}/retry - response includes updated workflow", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"name": "Template For Retry Response",
			"steps": []interface{}{
				map[string]interface{}{"name": "step1", "action": "test"},
			},
		})
		cr, err := http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		var tmpl map[string]interface{}
		json.NewDecoder(cr.Body).Decode(&tmpl)
		cr.Body.Close()
		tplID := tmpl["id"].(string)

		workflowBody, _ := json.Marshal(map[string]interface{}{
			"templateId": tplID,
			"tenant":     "test-tenant",
		})
		resp, err := http.Post(srv.URL+"/api/v1/workflows", "application/json", bytes.NewReader(workflowBody))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		var workflow map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&workflow)
		resp.Body.Close()
		workflowID := workflow["id"].(string)

		// Cancel to set failed state
		cancelReq, _ := http.NewRequest("POST", srv.URL+"/api/v1/workflows/"+workflowID+"/cancel", nil)
		http.DefaultClient.Do(cancelReq)

		// Retry and verify response includes the workflow
		retryReq, _ := http.NewRequest("POST", srv.URL+"/api/v1/workflows/"+workflowID+"/retry", nil)
		resp, _ = http.DefaultClient.Do(retryReq)
		var retryResult map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&retryResult)
		resp.Body.Close()
		if retryResult["id"] != workflowID {
			t.Errorf("expected workflow ID %s, got %v", workflowID, retryResult["id"])
		}
	})

	t.Run("POST /api/v1/workflows/{id}/cancel - cannot cancel already succeeded", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"name": "Template Already Succeeded",
			"steps": []interface{}{
				map[string]interface{}{"name": "step1", "action": "test"},
			},
		})
		cr, err := http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		var tmpl map[string]interface{}
		json.NewDecoder(cr.Body).Decode(&tmpl)
		cr.Body.Close()
		tplID := tmpl["id"].(string)

		workflowBody, _ := json.Marshal(map[string]interface{}{
			"templateId": tplID,
			"tenant":     "test-tenant",
		})
		resp, err := http.Post(srv.URL+"/api/v1/workflows", "application/json", bytes.NewReader(workflowBody))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		var workflow map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&workflow)
		resp.Body.Close()
		workflowID := workflow["id"].(string)

		// Workflow starts in pending, verify we can initially cancel
		cancelReq, _ := http.NewRequest("POST", srv.URL+"/api/v1/workflows/"+workflowID+"/cancel", nil)
		resp, err = http.DefaultClient.Do(cancelReq)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()

		// Now try to cancel again (should fail since already failed/cancelled)
		cancelReq2, _ := http.NewRequest("POST", srv.URL+"/api/v1/workflows/"+workflowID+"/cancel", nil)
		resp, err = http.DefaultClient.Do(cancelReq2)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for second cancel, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/templates/{id} - verify response content-type", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"name": "Content Type Check",
			"steps": []interface{}{
				map[string]interface{}{"name": "step1", "action": "test"},
			},
		})
		cr, _ := http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))
		var tmpl map[string]interface{}
		json.NewDecoder(cr.Body).Decode(&tmpl)
		cr.Body.Close()
		tplID := tmpl["id"].(string)

		resp, err := http.Get(srv.URL + "/api/v1/templates/" + tplID)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected application/json, got %s", ct)
		}
	})

	t.Run("POST /api/v1/workflows - verify Location header and content-type", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"name": "Location Header Test",
			"steps": []interface{}{
				map[string]interface{}{"name": "step1", "action": "test"},
			},
		})
		cr, err := http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		var tmpl map[string]interface{}
		json.NewDecoder(cr.Body).Decode(&tmpl)
		cr.Body.Close()
		tplID := tmpl["id"].(string)

		workflowBody, _ := json.Marshal(map[string]interface{}{
			"templateId": tplID,
			"tenant":     "test-tenant",
		})
		resp, err := http.Post(srv.URL+"/api/v1/workflows", "application/json", bytes.NewReader(workflowBody))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		// Verify Location header present
		if loc := resp.Header.Get("Location"); loc == "" {
			t.Error("expected Location header in response")
		}

		// Verify Content-Type
		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected application/json, got %s", ct)
		}

		// Verify status code is 202 Accepted
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("expected 202, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/workflows/{id} - verify response content-type", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"name": "Workflow Get Content Type",
			"steps": []interface{}{
				map[string]interface{}{"name": "step1", "action": "test"},
			},
		})
		cr, err := http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		var tmpl map[string]interface{}
		json.NewDecoder(cr.Body).Decode(&tmpl)
		cr.Body.Close()
		tplID := tmpl["id"].(string)

		workflowBody, _ := json.Marshal(map[string]interface{}{
			"templateId": tplID,
			"tenant":     "test-tenant",
		})
		resp, err := http.Post(srv.URL+"/api/v1/workflows", "application/json", bytes.NewReader(workflowBody))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		var workflow map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&workflow)
		resp.Body.Close()
		workflowID := workflow["id"].(string)

		resp, err = http.Get(srv.URL + "/api/v1/workflows/" + workflowID)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected application/json, got %s", ct)
		}
	})

	t.Run("POST /api/v1/workflows/{id}/cancel - verify response content-type and status", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"name": "Cancel Response Check",
			"steps": []interface{}{
				map[string]interface{}{"name": "step1", "action": "test"},
			},
		})
		cr, err := http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		var tmpl map[string]interface{}
		json.NewDecoder(cr.Body).Decode(&tmpl)
		cr.Body.Close()
		tplID := tmpl["id"].(string)

		workflowBody, _ := json.Marshal(map[string]interface{}{
			"templateId": tplID,
			"tenant":     "test-tenant",
		})
		resp, err := http.Post(srv.URL+"/api/v1/workflows", "application/json", bytes.NewReader(workflowBody))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		var workflow map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&workflow)
		resp.Body.Close()
		workflowID := workflow["id"].(string)

		cancelReq, _ := http.NewRequest("POST", srv.URL+"/api/v1/workflows/"+workflowID+"/cancel", nil)
		resp, err = http.DefaultClient.Do(cancelReq)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected application/json, got %s", ct)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/workflows/{id}/retry - verify response content-type and status", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"name": "Retry Response Check",
			"steps": []interface{}{
				map[string]interface{}{"name": "step1", "action": "test"},
			},
		})
		cr, err := http.Post(srv.URL+"/api/v1/templates", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		var tmpl map[string]interface{}
		json.NewDecoder(cr.Body).Decode(&tmpl)
		cr.Body.Close()
		tplID := tmpl["id"].(string)

		workflowBody, _ := json.Marshal(map[string]interface{}{
			"templateId": tplID,
			"tenant":     "test-tenant",
		})
		resp, err := http.Post(srv.URL+"/api/v1/workflows", "application/json", bytes.NewReader(workflowBody))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		var workflow map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&workflow)
		resp.Body.Close()
		workflowID := workflow["id"].(string)

		// Cancel to set failed state
		cancelReq, _ := http.NewRequest("POST", srv.URL+"/api/v1/workflows/"+workflowID+"/cancel", nil)
		http.DefaultClient.Do(cancelReq)

		retryReq, _ := http.NewRequest("POST", srv.URL+"/api/v1/workflows/"+workflowID+"/retry", nil)
		resp, err = http.DefaultClient.Do(retryReq)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected application/json, got %s", ct)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})
}
