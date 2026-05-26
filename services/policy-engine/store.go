package main

import (
	"encoding/json"
	"time"

	"github.com/penguintechinc/nest/shared/database"
	"gorm.io/datatypes"
)

// PolicyRule defines an access/retention/residency/DLP policy
type PolicyRule struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Tenant      string    `json:"tenant"`
	Labels      []string  `json:"labels"`                // which labels trigger this rule
	Action      string    `json:"action"`                // allow, deny, redact, warn
	Scope       string    `json:"scope,omitempty"`       // required scope (e.g. "data-access:pii")
	Regions     []string  `json:"regions,omitempty"`     // allowed regions (empty = any)
	Description string    `json:"description,omitempty"`
	Priority    int       `json:"priority"`              // higher = checked first
	CreatedAt   time.Time `json:"createdAt"`
}

// PolicyDecision is the result of evaluating policies for a resource
type PolicyDecision struct {
	ResourceID  string    `json:"resourceId"`
	UserRole    string    `json:"userRole"`
	Scope       string    `json:"requestedScope"`
	Decision    string    `json:"decision"`        // allow, deny, redact, warn
	Labels      []string  `json:"labelsApplied"`
	MatchedRule string    `json:"matchedRule,omitempty"`
	Reason      string    `json:"reason"`
	EvaluatedAt time.Time `json:"evaluatedAt"`
}

// PolicyStore manages policy rules using PenguinDAL
type PolicyStore struct {
	dal *database.PenguinDAL
}

func NewPolicyStore(dal *database.PenguinDAL) *PolicyStore {
	ps := &PolicyStore{
		dal: dal,
	}
	dal.DefineTable(&database.PolicyRule{})
	ps.seedDefaultPolicies()
	return ps
}

func (ps *PolicyStore) seedDefaultPolicies() {
	var count int64
	ps.dal.Query().Model(&database.PolicyRule{}).Count(&count)
	if count > 0 {
		return
	}

	defaults := []database.PolicyRule{
		{
			ID:          "policy-default-pii",
			Name:        "pii-scope-required",
			Labels:      datatypes.JSON(`["PII"]`),
			Action:      "deny",
			Scope:       "data-access:pii",
			Priority:    100,
			Description: "PII data requires data-access:pii scope",
			CreatedAt:   time.Now(),
		},
		{
			ID:          "policy-default-pci",
			Name:        "pci-us-only",
			Labels:      datatypes.JSON(`["PCI"]`),
			Action:      "deny",
			Regions:     datatypes.JSON(`["us-east-1", "us-west-2"]`),
			Priority:    90,
			Description: "PCI data restricted to US regions only",
			CreatedAt:   time.Now(),
		},
		{
			ID:          "policy-default-creds",
			Name:        "credentials-deny-all",
			Labels:      datatypes.JSON(`["CREDENTIALS"]`),
			Action:      "deny",
			Priority:    80,
			Description: "Credentials cannot be accessed",
			CreatedAt:   time.Now(),
		},
	}

	for _, rule := range defaults {
		ps.dal.Insert(&rule)
	}
}

// CreateRule creates a new policy rule
func (ps *PolicyStore) CreateRule(r *PolicyRule) (*PolicyRule, error) {
	labels, _ := json.Marshal(r.Labels)
	regions, _ := json.Marshal(r.Regions)

	dbRule := &database.PolicyRule{
		ID:          ps.dal.UUID(),
		Name:        r.Name,
		Tenant:      r.Tenant,
		Labels:      datatypes.JSON(labels),
		Action:      r.Action,
		Scope:       r.Scope,
		Regions:     datatypes.JSON(regions),
		Description: r.Description,
		Priority:    r.Priority,
		CreatedAt:   time.Now(),
	}

	if err := ps.dal.Insert(dbRule); err != nil {
		return nil, err
	}

	r.ID = dbRule.ID
	r.CreatedAt = dbRule.CreatedAt
	return r, nil
}

// GetRule retrieves a rule by ID
func (ps *PolicyStore) GetRule(id string) (*PolicyRule, bool) {
	var dbRule database.PolicyRule
	if err := ps.dal.Get(&dbRule, id); err != nil {
		return nil, false
	}

	var labels, regions []string
	json.Unmarshal(dbRule.Labels, &labels)
	json.Unmarshal(dbRule.Regions, &regions)

	return &PolicyRule{
		ID:          dbRule.ID,
		Name:        dbRule.Name,
		Tenant:      dbRule.Tenant,
		Labels:      labels,
		Action:      dbRule.Action,
		Scope:       dbRule.Scope,
		Regions:     regions,
		Description: dbRule.Description,
		Priority:    dbRule.Priority,
		CreatedAt:   dbRule.CreatedAt,
	}, true
}

// ListRules lists rules filtered by tenant
func (ps *PolicyStore) ListRules(tenant string) []*PolicyRule {
	var dbRules []database.PolicyRule
	query := ps.dal.Query()
	if tenant != "" {
		query = query.Where("tenant = ? OR tenant = ''", tenant)
	}
	query.Find(&dbRules)

	result := make([]*PolicyRule, len(dbRules))
	for i, dbRule := range dbRules {
		var labels, regions []string
		json.Unmarshal(dbRule.Labels, &labels)
		json.Unmarshal(dbRule.Regions, &regions)

		result[i] = &PolicyRule{
			ID:          dbRule.ID,
			Name:        dbRule.Name,
			Tenant:      dbRule.Tenant,
			Labels:      labels,
			Action:      dbRule.Action,
			Scope:       dbRule.Scope,
			Regions:     regions,
			Description: dbRule.Description,
			Priority:    dbRule.Priority,
			CreatedAt:   dbRule.CreatedAt,
		}
	}
	return result
}

// DeleteRule deletes a rule by ID
func (ps *PolicyStore) DeleteRule(id string) bool {
	if err := ps.dal.Delete(&database.PolicyRule{}, id); err != nil {
		return false
	}
	return true
}

// Evaluate evaluates policies for a resource
func (ps *PolicyStore) Evaluate(resourceID, userRole, requestedScope, region string, labels []string) *PolicyDecision {
	var dbRules []database.PolicyRule
	// In a real implementation, we'd query for rules that match any of the labels.
	// For simplicity, we list all and filter in memory as before, or use a complex query.
	ps.dal.Query().Order("priority desc").Find(&dbRules)

	decision := &PolicyDecision{
		ResourceID:  resourceID,
		UserRole:    userRole,
		Scope:       requestedScope,
		Decision:    "allow",
		Labels:      labels,
		Reason:      "no applicable policy",
		EvaluatedAt: time.Now(),
	}

	for _, dbRule := range dbRules {
		var ruleLabels []string
		json.Unmarshal(dbRule.Labels, &ruleLabels)

		if !ps.labelsIntersect(ruleLabels, labels) {
			continue
		}

		if dbRule.Scope != "" && !contains(requestedScope, dbRule.Scope) {
			decision.Decision = "deny"
			decision.Reason = "missing required scope " + dbRule.Scope
			decision.MatchedRule = dbRule.ID
			return decision
		}

		var ruleRegions []string
		json.Unmarshal(dbRule.Regions, &ruleRegions)
		if len(ruleRegions) > 0 && !stringSliceContains(ruleRegions, region) {
			decision.Decision = "deny"
			decision.Reason = "data residency violation"
			decision.MatchedRule = dbRule.ID
			return decision
		}

		if dbRule.Action != "allow" {
			decision.Decision = dbRule.Action
			decision.Reason = dbRule.Description
			if decision.Reason == "" && dbRule.Action == "redact" {
				decision.Reason = "column value redacted per policy"
			}
			decision.MatchedRule = dbRule.ID
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

func contains(scope, required string) bool {
	return scope == required || len(scope) > 0
}

func stringSliceContains(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}
