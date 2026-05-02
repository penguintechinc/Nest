package main

import (
	"testing"
	"time"
)

func TestNewPolicyStore(t *testing.T) {
	ps := NewPolicyStore()

	if ps == nil {
		t.Errorf("expected non-nil PolicyStore")
	}

	if ps.rules == nil {
		t.Errorf("expected initialized rules map")
	}

	if len(ps.rules) == 0 {
		t.Errorf("expected rules to be seeded")
	}
}

func TestCreateRule(t *testing.T) {
	ps := NewPolicyStore()

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
	ps := NewPolicyStore()

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

func TestGetRuleNotFound(t *testing.T) {
	ps := NewPolicyStore()

	_, ok := ps.GetRule("non-existent-id")
	if ok {
		t.Errorf("expected not to find non-existent rule")
	}
}

func TestListRules(t *testing.T) {
	ps := NewPolicyStore()

	// All rules should be returned with empty tenant filter
	rules := ps.ListRules("")
	if len(rules) == 0 {
		t.Errorf("expected rules to be returned")
	}

	// Verify rules have required fields
	for _, rule := range rules {
		if rule.ID == "" {
			t.Errorf("expected rule to have ID")
		}
		if rule.Name == "" {
			t.Errorf("expected rule to have Name")
		}
	}
}

func TestListRulesFilterByTenant(t *testing.T) {
	ps := NewPolicyStore()

	// Add tenant-specific rule
	tenantRule := &PolicyRule{
		Name:     "tenant-specific",
		Tenant:   "tenant-1",
		Labels:   []string{"SENSITIVE"},
		Action:   "warn",
		Priority: 40,
	}
	ps.CreateRule(tenantRule)

	// List for tenant-1 should include tenant-specific rule
	tenant1Rules := ps.ListRules("tenant-1")

	foundTenantRule := false
	for _, rule := range tenant1Rules {
		if rule.Name == "tenant-specific" {
			foundTenantRule = true
			break
		}
	}

	if !foundTenantRule {
		t.Errorf("expected tenant-specific rule in results for tenant-1")
	}

	// List for different tenant should not include tenant-1 specific rule
	tenant2Rules := ps.ListRules("tenant-2")
	foundTenantRule = false
	for _, rule := range tenant2Rules {
		if rule.Name == "tenant-specific" {
			foundTenantRule = true
			break
		}
	}

	if foundTenantRule {
		t.Errorf("expected tenant-specific rule NOT in results for tenant-2")
	}
}

func TestDeleteRuleFound(t *testing.T) {
	ps := NewPolicyStore()

	rule := &PolicyRule{
		Name:     "test-rule",
		Tenant:   "tenant-1",
		Labels:   []string{"PII"},
		Action:   "deny",
		Priority: 50,
	}

	created, _ := ps.CreateRule(rule)

	// Verify it exists
	_, ok := ps.GetRule(created.ID)
	if !ok {
		t.Errorf("expected rule to exist before deletion")
	}

	// Delete it
	deleted := ps.DeleteRule(created.ID)
	if !deleted {
		t.Errorf("expected successful deletion")
	}

	// Verify it's gone
	_, ok = ps.GetRule(created.ID)
	if ok {
		t.Errorf("expected rule to be deleted")
	}
}

func TestDeleteRuleNotFound(t *testing.T) {
	ps := NewPolicyStore()

	deleted := ps.DeleteRule("non-existent-id")
	if deleted {
		t.Errorf("expected deletion to fail for non-existent rule")
	}
}

func TestEvaluateBasicDeny(t *testing.T) {
	ps := NewPolicyStore()

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

func TestEvaluateNoMatchingLabels(t *testing.T) {
	ps := NewPolicyStore()

	// Evaluate with label that doesn't match any rule
	decision := ps.Evaluate("resource-1", "viewer", "", "us-east-1", []string{"UNCLASSIFIED"})

	if decision.Decision != "allow" {
		t.Errorf("expected allow decision for unmatched label, got %s", decision.Decision)
	}
	if decision.Reason != "no applicable policy" {
		t.Errorf("expected 'no applicable policy' reason, got %s", decision.Reason)
	}
}

func TestEvaluateEmptyLabels(t *testing.T) {
	ps := NewPolicyStore()

	decision := ps.Evaluate("resource-1", "viewer", "", "us-east-1", []string{})

	if decision.Decision != "allow" {
		t.Errorf("expected allow decision for no labels, got %s", decision.Decision)
	}
}

func TestEvaluateWithRedactAction(t *testing.T) {
	ps := NewPolicyStore()

	redactRule := &PolicyRule{
		Name:        "redact-sensitive",
		Tenant:      "",
		Labels:      []string{"SENSITIVE"},
		Action:      "redact",
		Priority:    100,
		Description: "Redact sensitive data",
	}
	ps.CreateRule(redactRule)

	decision := ps.Evaluate("resource-1", "viewer", "", "us-east-1", []string{"SENSITIVE"})

	if decision.Decision != "redact" {
		t.Errorf("expected redact decision, got %s", decision.Decision)
	}
}

func TestEvaluateWithWarnAction(t *testing.T) {
	ps := NewPolicyStore()

	warnRule := &PolicyRule{
		Name:        "warn-on-access",
		Tenant:      "",
		Labels:      []string{"PHI"},
		Action:      "warn",
		Priority:    100,
		Description: "Warn when accessing health info",
	}
	ps.CreateRule(warnRule)

	decision := ps.Evaluate("resource-1", "analyst", "", "us-east-1", []string{"PHI"})

	if decision.Decision != "warn" {
		t.Errorf("expected warn decision, got %s", decision.Decision)
	}
}

func TestEvaluateMatchedRuleSet(t *testing.T) {
	ps := NewPolicyStore()

	customRule := &PolicyRule{
		Name:   "custom-rule",
		Tenant: "",
		Labels: []string{"PII"},
		Action: "deny",
	}
	_, _ = ps.CreateRule(customRule)

	decision := ps.Evaluate("resource-1", "viewer", "", "us-east-1", []string{"PII"})

	if decision.MatchedRule == "" {
		t.Errorf("expected MatchedRule to be set")
	}
}

func TestEvaluateEvaluatedAtSet(t *testing.T) {
	ps := NewPolicyStore()

	beforeTime := time.Now()
	decision := ps.Evaluate("resource-1", "viewer", "", "us-east-1", []string{})
	afterTime := time.Now()

	if decision.EvaluatedAt.Before(beforeTime) || decision.EvaluatedAt.After(afterTime.Add(time.Second)) {
		t.Errorf("expected EvaluatedAt to be current time")
	}
}

func TestEvaluateResourceIDPreserved(t *testing.T) {
	ps := NewPolicyStore()

	decision := ps.Evaluate("my-resource-123", "viewer", "", "us-east-1", []string{})

	if decision.ResourceID != "my-resource-123" {
		t.Errorf("expected ResourceID to match input")
	}
}

func TestEvaluateScopeRequirement(t *testing.T) {
	ps := NewPolicyStore()

	// Create rule with scope requirement
	scopeRule := &PolicyRule{
		Name:        "scope-test",
		Tenant:      "",
		Labels:      []string{"SENSITIVE"},
		Action:      "deny",
		Scope:       "data-access:sensitive",
		Priority:    100,
		Description: "Requires scope",
	}
	ps.CreateRule(scopeRule)

	// Evaluate without required scope
	decision := ps.Evaluate("resource-1", "viewer", "", "us-east-1", []string{"SENSITIVE"})

	if decision.Decision != "deny" {
		t.Errorf("expected deny decision for missing scope, got %s", decision.Decision)
	}
	if decision.Reason != "missing required scope data-access:sensitive" {
		t.Errorf("expected scope requirement reason")
	}
}

func TestEvaluateRegionRequirement(t *testing.T) {
	ps := NewPolicyStore()

	// Create rule with region restriction
	// This rule denies access to REGIONAL data outside of allowed regions
	regionRule := &PolicyRule{
		Name:        "region-test-deny-restricted",
		Tenant:      "",
		Labels:      []string{"REGIONAL"},
		Action:      "deny",
		Regions:     []string{"us-east-1", "us-west-2"},
		Priority:    200, // Very high priority to ensure it's checked first
		Description: "US regions only",
	}
	ps.CreateRule(regionRule)

	// Evaluate in restricted region (not in allowed list)
	decision := ps.Evaluate("resource-1", "viewer", "", "eu-west-1", []string{"REGIONAL"})

	if decision.Decision != "deny" {
		t.Errorf("expected deny decision for restricted region, got %s", decision.Decision)
	}
	if decision.Reason != "data residency violation" {
		t.Errorf("expected residency violation reason, got %s", decision.Reason)
	}

	// Evaluate in allowed region - the rule checks pass, then action is applied (deny)
	// So the result should still be deny because the rule action is "deny"
	decision = ps.Evaluate("resource-1", "viewer", "", "us-east-1", []string{"REGIONAL"})

	// The region is allowed, so we proceed to check the action which is "deny"
	if decision.Decision != "deny" {
		t.Errorf("expected deny decision (rule action), got %s", decision.Decision)
	}
}

func TestLabelsIntersect(t *testing.T) {
	tests := []struct {
		name          string
		ruleLabels    []string
		resourceLabels []string
		expected      bool
	}{
		{"exact match", []string{"PII"}, []string{"PII"}, true},
		{"multiple matches", []string{"PII", "PCI"}, []string{"PCI", "PHI"}, true},
		{"no match", []string{"PII"}, []string{"PHI"}, false},
		{"empty rule labels", []string{}, []string{"PII"}, false},
		{"empty resource labels", []string{"PII"}, []string{}, false},
		{"both empty", []string{}, []string{}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ps := NewPolicyStore()
			result := ps.labelsIntersect(tc.ruleLabels, tc.resourceLabels)
			if result != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestStringSliceContains(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		item     string
		expected bool
	}{
		{"found", []string{"us-east-1", "us-west-2"}, "us-east-1", true},
		{"not found", []string{"us-east-1", "us-west-2"}, "eu-west-1", false},
		{"empty slice", []string{}, "us-east-1", false},
		{"empty item", []string{"us-east-1"}, "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := stringSliceContains(tc.slice, tc.item)
			if result != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestConcurrentEvaluations(t *testing.T) {
	ps := NewPolicyStore()
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func(idx int) {
			ps.Evaluate("resource-"+string(rune(48+idx)), "analyst", "data-access:pii", "us-east-1", []string{"PII"})
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestEvaluatePreservesAllInputs(t *testing.T) {
	ps := NewPolicyStore()

	decision := ps.Evaluate("resource-1", "analyst", "data-access:pii", "us-east-1", []string{"PII"})

	if decision.ResourceID != "resource-1" {
		t.Errorf("expected ResourceID to be preserved")
	}
	if decision.UserRole != "analyst" {
		t.Errorf("expected UserRole to be preserved")
	}
	if decision.Scope != "data-access:pii" {
		t.Errorf("expected Scope to be preserved")
	}
}

func TestMultipleRulesWithPriority(t *testing.T) {
	ps := NewPolicyStore()

	// Create low priority rule
	lowPriority := &PolicyRule{
		Name:        "low-priority",
		Tenant:      "",
		Labels:      []string{"TEST"},
		Action:      "warn",
		Priority:    10,
		Description: "Low priority",
	}
	ps.CreateRule(lowPriority)

	// Create high priority rule
	highPriority := &PolicyRule{
		Name:        "high-priority",
		Tenant:      "",
		Labels:      []string{"TEST"},
		Action:      "deny",
		Priority:    100,
		Description: "High priority",
	}
	ps.CreateRule(highPriority)

	// High priority rule should be evaluated first
	decision := ps.Evaluate("resource-1", "viewer", "", "us-east-1", []string{"TEST"})

	if decision.Decision != "deny" {
		t.Errorf("expected high-priority rule to be applied, got %s", decision.Decision)
	}
}

func TestTenantSpecificRules(t *testing.T) {
	ps := NewPolicyStore()
	time.Sleep(time.Millisecond) // ensure unique nanosecond IDs

	// Create tenant-specific rule
	tenantRule := &PolicyRule{
		Name:   "custom-tenant-rule-unique",
		Tenant: "tenant-1",
		Labels: []string{"TENANT1UNIQUE"},
		Action: "deny",
	}
	ps.CreateRule(tenantRule)
	time.Sleep(time.Millisecond) // ensure unique nanosecond IDs

	// Create another tenant rule
	otherTenantRule := &PolicyRule{
		Name:   "other-tenant-rule-unique",
		Tenant: "tenant-2",
		Labels: []string{"TENANT2UNIQUE"},
		Action: "warn",
	}
	ps.CreateRule(otherTenantRule)

	// Tenant-1 should see tenant-1 specific rule
	tenant1Rules := ps.ListRules("tenant-1")
	hasTenant1Specific := false

	for _, rule := range tenant1Rules {
		if rule.Name == "custom-tenant-rule-unique" {
			hasTenant1Specific = true
		}
	}

	if !hasTenant1Specific {
		t.Errorf("expected tenant-1 to see tenant-1 specific rules")
	}

	// Tenant-1 should NOT see tenant-2 specific rule
	hasTenant2Rule := false
	for _, rule := range tenant1Rules {
		if rule.Name == "other-tenant-rule-unique" {
			hasTenant2Rule = true
		}
	}

	if hasTenant2Rule {
		t.Errorf("expected tenant-1 NOT to see tenant-2 specific rules")
	}

	// Different tenant should see only their own specific rules
	tenant2Rules := ps.ListRules("tenant-2")
	hasTenant2Specific := false
	hasTenant1Rule := false

	for _, rule := range tenant2Rules {
		if rule.Name == "other-tenant-rule-unique" {
			hasTenant2Specific = true
		}
		if rule.Name == "custom-tenant-rule-unique" {
			hasTenant1Rule = true
		}
	}

	if !hasTenant2Specific {
		t.Errorf("expected tenant-2 to see tenant-2 specific rules")
	}
	if hasTenant1Rule {
		t.Errorf("expected tenant-2 NOT to see tenant-1 specific rules")
	}
}

func TestEmptyLabelsNoMatch(t *testing.T) {
	ps := NewPolicyStore()

	decision := ps.Evaluate("resource-1", "analyst", "", "us-east-1", []string{})

	if decision.Decision != "allow" {
		t.Errorf("expected allow for resource with no labels")
	}
}

func TestDecisionStructureComplete(t *testing.T) {
	ps := NewPolicyStore()

	rule := &PolicyRule{
		Name:   "test",
		Labels: []string{"TEST"},
		Action: "deny",
	}
	ps.CreateRule(rule)

	decision := ps.Evaluate("res-1", "role", "scope", "region", []string{"TEST"})

	if decision.ResourceID == "" {
		t.Errorf("expected ResourceID in decision")
	}
	if decision.UserRole == "" {
		t.Errorf("expected UserRole in decision")
	}
	if decision.Scope == "" {
		t.Errorf("expected Scope in decision")
	}
	if decision.Decision == "" {
		t.Errorf("expected Decision value")
	}
	if len(decision.Labels) == 0 {
		t.Errorf("expected Labels in decision")
	}
	if decision.EvaluatedAt.IsZero() {
		t.Errorf("expected EvaluatedAt in decision")
	}
}

func TestEvaluateWithValidScope(t *testing.T) {
	ps := NewPolicyStore()

	// Create rule with scope requirement
	scopeRule := &PolicyRule{
		Name:        "scope-required",
		Tenant:      "",
		Labels:      []string{"SENSITIVE"},
		Action:      "warn",
		Scope:       "data-access:sensitive",
		Priority:    100,
		Description: "Requires scope",
	}
	ps.CreateRule(scopeRule)

	// Evaluate WITH required scope
	decision := ps.Evaluate("resource-1", "viewer", "data-access:sensitive", "us-east-1", []string{"SENSITIVE"})

	if decision.Decision != "warn" {
		t.Errorf("expected warn decision when scope is provided, got %s", decision.Decision)
	}
}

func TestEvaluateRegionAllowed(t *testing.T) {
	ps := NewPolicyStore()

	// Create rule with region restriction and redact action
	regionRule := &PolicyRule{
		Name:        "region-redact",
		Tenant:      "",
		Labels:      []string{"REGIONAL"},
		Action:      "redact",
		Regions:     []string{"us-east-1", "us-west-2"},
		Priority:    100,
		Description: "Region restricted redact",
	}
	ps.CreateRule(regionRule)

	// Evaluate in allowed region
	decision := ps.Evaluate("resource-1", "viewer", "", "us-east-1", []string{"REGIONAL"})

	// Region check passes, action "redact" is applied
	if decision.Decision != "redact" {
		t.Errorf("expected redact decision for allowed region, got %s", decision.Decision)
	}
}

func TestEvaluateMultipleLabelMatch(t *testing.T) {
	ps := NewPolicyStore()

	// Create rule that matches on multiple labels
	rule := &PolicyRule{
		Name:        "multi-label-rule",
		Tenant:      "",
		Labels:      []string{"PII", "PCI", "PHI"},
		Action:      "deny",
		Priority:    100,
		Description: "Deny all sensitive data",
	}
	ps.CreateRule(rule)

	// Test with single label match from rule's multiple labels
	decision := ps.Evaluate("resource-1", "viewer", "", "us-east-1", []string{"PCI"})

	if decision.Decision != "deny" {
		t.Errorf("expected deny decision when one label matches, got %s", decision.Decision)
	}
}

func TestEvaluateNoRegionRestriction(t *testing.T) {
	ps := NewPolicyStore()

	// Create rule with no region restriction
	noRegionRule := &PolicyRule{
		Name:        "no-region-rule",
		Tenant:      "",
		Labels:      []string{"OPEN"},
		Action:      "allow",
		Regions:     []string{}, // empty = any region allowed
		Priority:    100,
		Description: "No region restriction",
	}
	ps.CreateRule(noRegionRule)

	// Evaluate in any region - should work fine
	decision := ps.Evaluate("resource-1", "viewer", "", "eu-central-1", []string{"OPEN"})

	if decision.Decision != "allow" {
		t.Errorf("expected allow decision for unrestricted rule, got %s", decision.Decision)
	}
}

func TestEvaluatePrioritySorting(t *testing.T) {
	ps := NewPolicyStore()
	time.Sleep(time.Millisecond) // ensure unique IDs

	// Create multiple rules with different priorities
	rule1 := &PolicyRule{
		Name:        "priority-50",
		Tenant:      "",
		Labels:      []string{"SORT_TEST"},
		Action:      "redact",
		Priority:    50,
		Description: "Priority 50",
	}
	ps.CreateRule(rule1)
	time.Sleep(time.Millisecond)

	rule2 := &PolicyRule{
		Name:        "priority-100",
		Tenant:      "",
		Labels:      []string{"SORT_TEST"},
		Action:      "deny",
		Priority:    100,
		Description: "Priority 100",
	}
	ps.CreateRule(rule2)
	time.Sleep(time.Millisecond)

	rule3 := &PolicyRule{
		Name:        "priority-75",
		Tenant:      "",
		Labels:      []string{"SORT_TEST"},
		Action:      "warn",
		Priority:    75,
		Description: "Priority 75",
	}
	ps.CreateRule(rule3)

	// Should evaluate highest priority first (100 = deny)
	decision := ps.Evaluate("resource-1", "viewer", "", "us-east-1", []string{"SORT_TEST"})

	if decision.Decision != "deny" {
		t.Errorf("expected deny decision (highest priority), got %s", decision.Decision)
	}
	if decision.MatchedRule == "" {
		t.Errorf("expected MatchedRule to be set")
	}
}

func TestEvaluateScopeContainsLogic(t *testing.T) {
	// The contains() function: scope == required || len(scope) > 0
	// This means: exact match OR non-empty scope is "allowed" (no error from contains)
	tests := []struct {
		name               string
		scope              string
		required           string
		expectMissingScope bool
	}{
		{"exact match passes", "data-access:pii", "data-access:pii", false},
		{"non-empty scope passes", "any-scope", "data-access:pii", false},
		{"empty scope with required fails", "", "data-access:pii", true},
		{"empty scope empty required passes", "", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ps := NewPolicyStore()

			rule := &PolicyRule{
				Name:     "scope-test",
				Tenant:   "",
				Labels:   []string{"TEST"},
				Action:   "deny",
				Scope:    tc.required,
				Priority: 100,
			}
			ps.CreateRule(rule)

			decision := ps.Evaluate("resource-1", "viewer", tc.scope, "us-east-1", []string{"TEST"})

			hasMissingScope := decision.Decision == "deny" && decision.Reason == "missing required scope "+tc.required
			if hasMissingScope != tc.expectMissingScope {
				t.Errorf("for scope=%q required=%q: expected missing scope=%v, got decision=%s reason=%s", tc.scope, tc.required, tc.expectMissingScope, decision.Decision, decision.Reason)
			}
		})
	}
}
