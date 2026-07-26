package main

import (
	"testing"
)

func TestNewPolicyStore(t *testing.T) {
	dal := getTestDAL()
	ps := NewPolicyStore(dal)

	if ps == nil {
		t.Errorf("expected non-nil PolicyStore")
	}
}

func TestCreateRule(t *testing.T) {
	dal := getTestDAL()
	ps := NewPolicyStore(dal)

	newRule := &PolicyRule{
		Name:        "test-rule",
		Tenant:      "tenant-1",
		Labels:      []string{"PII"},
		Action:      "redact",
		Scope:       "data-access:pii",
		Priority:    50,
		Description: "Test rule",
	}

	created, err := ps.CreateRule(newRule)
	if err != nil {
		t.Errorf("expected no error creating rule, got %v", err)
	}

	if created.ID == "" {
		t.Errorf("expected ID to be assigned")
	}

	if created.CreatedAt.IsZero() {
		t.Errorf("expected CreatedAt to be set")
	}
}

func TestGetRuleFound(t *testing.T) {
	dal := getTestDAL()
	ps := NewPolicyStore(dal)

	newRule := &PolicyRule{
		Name:     "test-rule",
		Tenant:   "tenant-1",
		Labels:   []string{"PII"},
		Action:   "deny",
		Priority: 50,
	}

	created, _ := ps.CreateRule(newRule)

	retrieved, ok := ps.GetRule(created.ID)
	if !ok {
		t.Errorf("expected to find rule by ID")
	}

	if retrieved.Name != "test-rule" {
		t.Errorf("expected retrieved rule name to match")
	}
}

func TestListRules(t *testing.T) {
	dal := getTestDAL()
	ps := NewPolicyStore(dal)

	// All rules should be returned with empty tenant filter
	rules := ps.ListRules("")
	if len(rules) == 0 {
		t.Errorf("expected rules to be returned")
	}
}

func TestEvaluateBasicDeny(t *testing.T) {
	dal := getTestDAL()
	ps := NewPolicyStore(dal)

	// Create a simple deny rule
	denyRule := &PolicyRule{
		Name:        "test-deny",
		Tenant:      "",
		Labels:      []string{"CREDENTIALS"},
		Action:      "deny",
		Priority:    100,
		Description: "Test deny rule",
	}
	ps.CreateRule(denyRule)

	decision := ps.Evaluate("resource-1", "analyst", "", "us-east-1", []string{"CREDENTIALS"})

	if decision.Decision != "deny" {
		t.Errorf("expected deny decision, got %s", decision.Decision)
	}
}
