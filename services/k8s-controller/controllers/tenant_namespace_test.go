package controllers

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// A freshly created tenant namespace must carry nest.penguintech.io/tenant=<name>
// so cross-namespace NetworkPolicies can select it by label rather than by name.
func TestEnsureTenantNamespace_LabelsNewNamespace(t *testing.T) {
	r, ctx := reconcilerFor(t)

	if err := r.ensureTenantNamespace(ctx, "acme"); err != nil {
		t.Fatalf("ensureTenantNamespace() error = %v", err)
	}

	var ns corev1.Namespace
	if err := r.Client.Get(ctx, types.NamespacedName{Name: "acme"}, &ns); err != nil {
		t.Fatalf("namespace not created: %v", err)
	}
	if ns.Labels["nest.penguintech.io/tenant"] != "acme" {
		t.Errorf("tenant label = %q, want acme", ns.Labels["nest.penguintech.io/tenant"])
	}
	if ns.Labels["nest.penguintech.io/managed"] != "true" {
		t.Errorf("managed label = %q, want true", ns.Labels["nest.penguintech.io/managed"])
	}
}

// A namespace that predates the tenant label (created out-of-band) must be
// backfilled on reconcile, without clobbering operator-set labels.
func TestEnsureTenantNamespace_BackfillsExistingNamespace(t *testing.T) {
	existing := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "legacy",
			Labels: map[string]string{"team": "platform"},
		},
	}
	r, ctx := reconcilerFor(t, existing)

	if err := r.ensureTenantNamespace(ctx, "legacy"); err != nil {
		t.Fatalf("ensureTenantNamespace() error = %v", err)
	}

	var ns corev1.Namespace
	if err := r.Client.Get(ctx, types.NamespacedName{Name: "legacy"}, &ns); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if ns.Labels["nest.penguintech.io/tenant"] != "legacy" {
		t.Errorf("tenant label = %q, want legacy (not backfilled)", ns.Labels["nest.penguintech.io/tenant"])
	}
	if ns.Labels["team"] != "platform" {
		t.Errorf("operator label 'team' = %q, want platform (must not be clobbered)", ns.Labels["team"])
	}
}

// Reconciling a namespace that already carries the labels must not error and must
// leave them intact (idempotent).
func TestEnsureTenantNamespace_Idempotent(t *testing.T) {
	existing := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "acme",
			Labels: map[string]string{
				"nest.penguintech.io/managed": "true",
				"nest.penguintech.io/tenant":  "acme",
			},
		},
	}
	r, ctx := reconcilerFor(t, existing)

	if err := r.ensureTenantNamespace(ctx, "acme"); err != nil {
		t.Fatalf("ensureTenantNamespace() error = %v", err)
	}

	var ns corev1.Namespace
	if err := r.Client.Get(ctx, types.NamespacedName{Name: "acme"}, &ns); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if ns.Labels["nest.penguintech.io/tenant"] != "acme" {
		t.Errorf("tenant label = %q, want acme", ns.Labels["nest.penguintech.io/tenant"])
	}
}

// Reconciling a tenant namespace must create a default-deny NetworkPolicy that
// restricts egress to controller namespace to only gateway service pods (by app.kubernetes.io/name label).
func TestEnsureTenantNetworkPolicies_GatewaySelectorLabels(t *testing.T) {
	r, ctx := reconcilerFor(t)

	if err := r.ensureTenantNetworkPolicies(ctx, "acme"); err != nil {
		t.Fatalf("ensureTenantNetworkPolicies() error = %v", err)
	}

	var policy networking.NetworkPolicy
	if err := r.Client.Get(ctx, types.NamespacedName{Name: "nest-default-deny", Namespace: "acme"}, &policy); err != nil {
		t.Fatalf("NetworkPolicy not created: %v", err)
	}

	// Find the egress rule that targets the controller namespace
	var controllerEgress *networking.NetworkPolicyEgressRule
	for i := range policy.Spec.Egress {
		rule := &policy.Spec.Egress[i]
		if len(rule.To) > 0 && rule.To[0].NamespaceSelector != nil {
			ns := rule.To[0].NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"]
			if ns == "nest-controller" {
				controllerEgress = rule
				break
			}
		}
	}
	if controllerEgress == nil {
		t.Fatal("no egress rule found targeting nest-controller namespace")
	}

	// Verify the controller rule has exactly one peer
	if len(controllerEgress.To) != 1 {
		t.Errorf("controller egress peer count = %d, want 1", len(controllerEgress.To))
	}
	peer := controllerEgress.To[0]

	// Verify NamespaceSelector is present
	if peer.NamespaceSelector == nil {
		t.Fatal("NamespaceSelector is nil, expected it to select nest-controller namespace")
	}
	if peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] != "nest-controller" {
		t.Errorf("NamespaceSelector namespace = %q, want nest-controller", peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"])
	}

	// Verify PodSelector is present and matches gateway services
	if peer.PodSelector == nil {
		t.Fatal("PodSelector is nil, expected it to restrict to gateway pods")
	}
	if len(peer.PodSelector.MatchExpressions) == 0 {
		t.Fatal("PodSelector has no matchExpressions")
	}

	// Find the app.kubernetes.io/name expression
	var nameExpr *metav1.LabelSelectorRequirement
	for i := range peer.PodSelector.MatchExpressions {
		expr := &peer.PodSelector.MatchExpressions[i]
		if expr.Key == "app.kubernetes.io/name" {
			nameExpr = expr
			break
		}
	}
	if nameExpr == nil {
		t.Fatal("no matchExpression found for app.kubernetes.io/name label")
	}

	// Verify operator is In
	if nameExpr.Operator != metav1.LabelSelectorOpIn {
		t.Errorf("operator = %q, want In", nameExpr.Operator)
	}

	// Verify all three gateway services are listed
	expectedGateways := map[string]bool{
		"nest-gateway":       false,
		"nest-iscsi-gateway": false,
		"nest-nfs-gateway":   false,
	}
	for _, val := range nameExpr.Values {
		if _, ok := expectedGateways[val]; !ok {
			t.Errorf("unexpected gateway value %q", val)
		}
		expectedGateways[val] = true
	}
	for gw, found := range expectedGateways {
		if !found {
			t.Errorf("missing gateway %q in PodSelector values", gw)
		}
	}
}
