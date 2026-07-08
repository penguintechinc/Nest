package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// PolicyRule defines an access/retention/residency/DLP policy
type PolicyRule struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Tenant      string    `json:"tenant"`
	Labels      []string  `json:"labels"`            // which labels trigger this rule
	Action      string    `json:"action"`            // allow, deny, redact, warn
	Scope       string    `json:"scope,omitempty"`   // required scope (e.g. "data-access:pii")
	Regions     []string  `json:"regions,omitempty"` // allowed regions (empty = any)
	Description string    `json:"description,omitempty"`
	Priority    int       `json:"priority"` // higher = checked first
	CreatedAt   time.Time `json:"createdAt"`
}

// PolicyDecision is the result of evaluating policies for a resource
type PolicyDecision struct {
	ResourceID  string    `json:"resourceId"`
	UserRole    string    `json:"userRole"`
	Scope       string    `json:"requestedScope"`
	Decision    string    `json:"decision"` // allow, deny, redact, warn
	Labels      []string  `json:"labelsApplied"`
	MatchedRule string    `json:"matchedRule,omitempty"`
	Reason      string    `json:"reason"`
	EvaluatedAt time.Time `json:"evaluatedAt"`
}

// PolicyStore manages policy rules
type PolicyStore struct {
	mu    sync.RWMutex
	rules map[string]*PolicyRule
}

func NewPolicyStore() *PolicyStore {
	ps := &PolicyStore{
		rules: make(map[string]*PolicyRule),
	}
	ps.seedDefaultPolicies()
	return ps
}

func (ps *PolicyStore) seedDefaultPolicies() {
	defaults := []*PolicyRule{
		{
			Name:        "pii-scope-required",
			Tenant:      "",
			Labels:      []string{"PII"},
			Action:      "deny",
			Scope:       "data-access:pii",
			Priority:    100,
			Description: "PII data requires data-access:pii scope",
			CreatedAt:   time.Now(),
		},
		{
			Name:        "pci-us-only",
			Tenant:      "",
			Labels:      []string{"PCI"},
			Action:      "deny",
			Regions:     []string{"us-east-1", "us-west-2"},
			Priority:    90,
			Description: "PCI data restricted to US regions only",
			CreatedAt:   time.Now(),
		},
		{
			Name:        "credentials-deny-all",
			Tenant:      "",
			Labels:      []string{"CREDENTIALS"},
			Action:      "deny",
			Priority:    80,
			Description: "Credentials cannot be accessed",
			CreatedAt:   time.Now(),
		},
	}

	for _, rule := range defaults {
		rule.ID = fmt.Sprintf("policy-%d", time.Now().UnixNano())
		ps.rules[rule.ID] = rule
	}
}

// CreateRule creates a new policy rule
func (ps *PolicyStore) CreateRule(r *PolicyRule) (*PolicyRule, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	r.ID = fmt.Sprintf("policy-%d", time.Now().UnixNano())
	r.CreatedAt = time.Now()
	ps.rules[r.ID] = r
	return r, nil
}

// GetRule retrieves a rule by ID
func (ps *PolicyStore) GetRule(id string) (*PolicyRule, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	r, ok := ps.rules[id]
	return r, ok
}

// ListRules lists rules filtered by tenant
func (ps *PolicyStore) ListRules(tenant string) []*PolicyRule {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	var result []*PolicyRule
	for _, rule := range ps.rules {
		if tenant == "" {
			result = append(result, rule)
		} else if rule.Tenant == "" || rule.Tenant == tenant {
			result = append(result, rule)
		}
	}
	return result
}

// DeleteRule deletes a rule by ID
func (ps *PolicyStore) DeleteRule(id string) bool {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if _, ok := ps.rules[id]; ok {
		delete(ps.rules, id)
		return true
	}
	return false
}

// Evaluate evaluates policies for a resource
func (ps *PolicyStore) Evaluate(resourceID, userRole, requestedScope, region string, labels []string) *PolicyDecision {
	ps.mu.RLock()

	// Collect matching rules
	var matching []*PolicyRule
	for _, rule := range ps.rules {
		if ps.labelsIntersect(rule.Labels, labels) {
			matching = append(matching, rule)
		}
	}
	ps.mu.RUnlock()

	// Sort by priority descending
	for i := 0; i < len(matching); i++ {
		for j := i + 1; j < len(matching); j++ {
			if matching[j].Priority > matching[i].Priority {
				matching[i], matching[j] = matching[j], matching[i]
			}
		}
	}

	decision := &PolicyDecision{
		ResourceID:  resourceID,
		UserRole:    userRole,
		Scope:       requestedScope,
		Decision:    "allow",
		Labels:      labels,
		Reason:      "no applicable policy",
		EvaluatedAt: time.Now(),
	}

	// Evaluate each rule in priority order
	for _, rule := range matching {
		if rule.Scope != "" && !contains(requestedScope, rule.Scope) {
			decision.Decision = "deny"
			decision.Reason = "missing required scope " + rule.Scope
			decision.MatchedRule = rule.ID
			return decision
		}
		if len(rule.Regions) > 0 && !stringSliceContains(rule.Regions, region) {
			decision.Decision = "deny"
			decision.Reason = "data residency violation"
			decision.MatchedRule = rule.ID
			return decision
		}
		if rule.Action == "deny" {
			decision.Decision = "deny"
			decision.Reason = rule.Description
			decision.MatchedRule = rule.ID
			return decision
		}
		if rule.Action == "redact" {
			decision.Decision = "redact"
			decision.Reason = "column value redacted per policy"
			decision.MatchedRule = rule.ID
			return decision
		}
		if rule.Action == "warn" {
			decision.Decision = "warn"
			decision.Reason = rule.Description
			decision.MatchedRule = rule.ID
			return decision
		}
	}

	return decision
}

func (ps *PolicyStore) labelsIntersect(ruleLabels, resourceLabels []string) bool {
	for _, rl := range ruleLabels {
		for _, xl := range resourceLabels {
			if rl == xl {
				return true
			}
		}
	}
	return false
}

// contains checks if a space-delimited scope list satisfies the required scope.
// Scope matching semantics:
// - Exact match: resource:action == required resource:action
// - Wildcard resource: *:action satisfies resource:action if action matches or is admin
// - Admin action: admin action satisfies any action (write satisfies read, etc.)
func contains(scope, required string) bool {
	if required == "" {
		return false
	}
	scopes := strings.Fields(scope)
	for _, s := range scopes {
		if scopeSatisfies(s, required) {
			return true
		}
	}
	return false
}

// scopeSatisfies checks if a single scope satisfies the required scope.
func scopeSatisfies(scope, required string) bool {
	// Parse scope and required into resource and action
	scopeParts := strings.Split(scope, ":")
	requiredParts := strings.Split(required, ":")

	if len(scopeParts) != 2 || len(requiredParts) != 2 {
		return false
	}

	scopeResource, scopeAction := scopeParts[0], scopeParts[1]
	requiredResource, requiredAction := requiredParts[0], requiredParts[1]

	// Resource matching: exact match or wildcard
	resourceMatches := scopeResource == requiredResource || scopeResource == "*"
	if !resourceMatches {
		return false
	}

	// Action matching: exact match or admin action covers everything
	actionMatches := scopeAction == requiredAction || scopeAction == "admin"
	return actionMatches
}

func stringSliceContains(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}
