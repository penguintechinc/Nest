package controllers

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
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
