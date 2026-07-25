package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// These tests pin the reconciler request bodies to the gateways' actual
// CreateTargetRequest / CreateExportRequest shapes. Both gateways bind JSON and
// ignore unknown keys, so a field sent under the wrong name is silently dropped
// rather than rejected — previously the reconcilers sent nested "acl"/"chap"
// objects and the targets were provisioned with no ACL and no CHAP auth at all.
// Asserting only on Status.Phase (as the older tests did) cannot catch that.

// captureGateway records the decoded body of the first request it serves.
func captureGateway(t *testing.T, respond func(w http.ResponseWriter)) (*httptest.Server, *map[string]interface{}) {
	t.Helper()
	captured := map[string]interface{}{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Errorf("reading request body: %v", err)
		}
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Errorf("decoding request body %q: %v", body, err)
		}
		respond(w)
	}))
	t.Cleanup(srv.Close)
	return srv, &captured
}

func TestISCSI_RequestMatchesGatewayContract(t *testing.T) {
	srv, captured := captureGateway(t, func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintln(w, `{"id":"target-contract","iqn":"iqn.2024-01.io.penguintech:target-contract"}`)
	})
	t.Setenv("ISCSI_GATEWAY_ENDPOINT", srv.URL)

	dr := newDR("iscsi-contract", "tenant-contract", "iscsi")
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	body := *captured

	// The initiator IQN is the ACL. It must reach the gateway under the exact key
	// its CreateTargetRequest binds, or the target admits any initiator.
	if got := body["initiatorIqn"]; got != "iqn.1993-08.org.debian:01:iscsi-contract" {
		t.Errorf("initiatorIqn = %v, want the DataResource's initiator IQN", got)
	}

	// CHAP credentials likewise.
	if got := body["chapUsername"]; got != "tenant-tenant-contract-iscsi-contract" {
		t.Errorf("chapUsername = %v, want tenant-scoped username", got)
	}
	pw, ok := body["chapPassword"].(string)
	if !ok || pw == "" {
		t.Fatalf("chapPassword missing from request body: %v", body["chapPassword"])
	}
	if pw == "generated-secret" {
		t.Error("chapPassword is the hardcoded placeholder, not a generated secret")
	}
	if len(pw) != iscsiCHAPPasswordLength {
		t.Errorf("chapPassword length = %d, want %d (Windows initiators reject outside 12-16)", len(pw), iscsiCHAPPasswordLength)
	}

	// The old nested shape must not reappear — the gateway would drop it.
	if _, exists := body["chap"]; exists {
		t.Error(`request contains nested "chap" object; the gateway ignores it`)
	}
	if _, exists := body["acl"]; exists {
		t.Error(`request contains nested "acl" object; the gateway ignores it`)
	}
}

// The CHAP password must be persisted so it survives reconciles, and must differ
// between resources — a shared secret would let any tenant attach another's LUN.
func TestISCSI_CHAPSecretIsPersistedAndUnique(t *testing.T) {
	srv, _ := captureGateway(t, func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintln(w, `{"id":"target-a","iqn":"iqn.2024-01.io.penguintech:target-a"}`)
	})
	t.Setenv("ISCSI_GATEWAY_ENDPOINT", srv.URL)

	drA := newDR("iscsi-a", "tenant-a", "iscsi")
	drB := newDR("iscsi-b", "tenant-b", "iscsi")
	r, ctx := reconcilerFor(t, drA, drB)

	if _, err := r.Reconcile(ctx, reqFor(drA)); err != nil {
		t.Fatalf("Reconcile(A) error = %v", err)
	}
	if _, err := r.Reconcile(ctx, reqFor(drB)); err != nil {
		t.Fatalf("Reconcile(B) error = %v", err)
	}

	readPW := func(dr *nestv1.DataResource) string {
		t.Helper()
		var s corev1.Secret
		key := types.NamespacedName{Name: iscsiCHAPSecretName(dr), Namespace: dr.Spec.Tenant}
		if err := r.Client.Get(ctx, key, &s); err != nil {
			t.Fatalf("CHAP secret %v not created: %v", key, err)
		}
		return string(s.Data["password"])
	}

	pwA, pwB := readPW(drA), readPW(drB)
	if pwA == "" || pwB == "" {
		t.Fatal("CHAP secret has empty password")
	}
	if pwA == pwB {
		t.Error("two DataResources share a CHAP password; each target must have its own")
	}
}

// A retry must not strand a duplicate target. The gateway mints a fresh ID per
// POST and has no server-side idempotency, so once the target is recorded the
// reconciler verifies it rather than creating a second one.
//
// The retry is driven by resetting the phase, which is the real failure mode: the
// target is created and the ID annotation persisted, but the controller dies (or
// the status write fails) before the resource reaches Ready. On restart it
// re-reconciles a resource that already owns a target.
func TestISCSI_ReconcileIsIdempotent(t *testing.T) {
	var posts, gets int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodPost:
			atomic.AddInt32(&posts, 1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintln(w, `{"id":"target-idem","iqn":"iqn.2024-01.io.penguintech:target-idem"}`)
		case http.MethodGet:
			atomic.AddInt32(&gets, 1)
			w.WriteHeader(http.StatusOK) // target still exists
			fmt.Fprintln(w, `{"id":"target-idem"}`)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()
	t.Setenv("ISCSI_GATEWAY_ENDPOINT", srv.URL)

	dr := newDR("iscsi-idem", "tenant-idem", "iscsi")
	r, ctx := reconcilerFor(t, dr)

	for i := 0; i < 3; i++ {
		if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
			t.Fatalf("Reconcile() iteration %d error = %v", i, err)
		}
		resetPhase(t, r, ctx, dr)
	}

	if got := atomic.LoadInt32(&posts); got != 1 {
		t.Errorf("gateway received %d POSTs across 3 reconciles, want 1 (duplicate targets stranded)", got)
	}
	if atomic.LoadInt32(&gets) == 0 {
		t.Error("reconciler never verified the existing target before recreating")
	}
}

// resetPhase drops the resource back to Pending so the next Reconcile runs the
// provisioning path instead of short-circuiting on Ready.
func resetPhase(t *testing.T, r *DataResourceReconciler, ctx context.Context, dr *nestv1.DataResource) {
	t.Helper()
	var cur nestv1.DataResource
	if err := r.Client.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &cur); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	cur.Status.Phase = nestv1.PhasePending
	if err := r.Client.Status().Update(ctx, &cur); err != nil {
		t.Fatalf("Status().Update() error = %v", err)
	}
}

// A target with no initiator IQN would admit any initiator, so provisioning must
// fail rather than create an open target.
func TestISCSI_MissingInitiatorFailsClosed(t *testing.T) {
	srv, _ := captureGateway(t, func(w http.ResponseWriter) {
		t.Error("gateway must not be called when the initiator IQN is missing")
		w.WriteHeader(http.StatusCreated)
	})
	t.Setenv("ISCSI_GATEWAY_ENDPOINT", srv.URL)

	dr := newDR("iscsi-open", "tenant-open", "iscsi")
	dr.Spec.Annotations = nil // no ACL scope

	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err == nil {
		t.Fatal("expected Reconcile to fail without an initiator IQN")
	}

	var updated nestv1.DataResource
	if err := r.Client.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if updated.Status.Phase != nestv1.PhaseFailed {
		t.Errorf("Status.Phase = %v, want Failed", updated.Status.Phase)
	}
}

func TestNFS_RequestMatchesGatewayContract(t *testing.T) {
	srv, captured := captureGateway(t, func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintln(w, `{"id":"export-contract"}`)
	})
	t.Setenv("NFS_GATEWAY_ENDPOINT", srv.URL)

	dr := newDR("nfs-contract", "tenant-nfs-contract", "nfs")
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	body := *captured

	// The client CIDR is the export's only access control. It must come from the
	// resource, never a broad hardcoded default.
	if got := body["clients"]; got != "10.42.0.0/16" {
		t.Errorf("clients = %v, want the DataResource's allowed-clients CIDR", got)
	}
	if got := body["clients"]; got == "10.0.0.0/8" || got == "*" {
		t.Error("export is open to the whole cluster; tenant isolation is broken")
	}
	if got := body["path"]; got != "/cephfs/nest/tenant-nfs-contract/nfs-contract" {
		t.Errorf("path = %v, want the tenant-scoped export path", got)
	}
}

// Same fail-closed contract as iSCSI: no client scope, no export.
func TestNFS_MissingAllowedClientsFailsClosed(t *testing.T) {
	srv, _ := captureGateway(t, func(w http.ResponseWriter) {
		t.Error("gateway must not be called when the allowed-clients CIDR is missing")
		w.WriteHeader(http.StatusCreated)
	})
	t.Setenv("NFS_GATEWAY_ENDPOINT", srv.URL)

	dr := newDR("nfs-open", "tenant-nfs-open", "nfs")
	dr.Spec.Annotations = nil // no access scope

	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err == nil {
		t.Fatal("expected Reconcile to fail without an allowed-clients CIDR")
	}

	var updated nestv1.DataResource
	if err := r.Client.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if updated.Status.Phase != nestv1.PhaseFailed {
		t.Errorf("Status.Phase = %v, want Failed", updated.Status.Phase)
	}
}

func TestNFS_ReconcileIsIdempotent(t *testing.T) {
	var posts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodPost:
			atomic.AddInt32(&posts, 1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintln(w, `{"id":"export-idem"}`)
		case http.MethodGet:
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, `{"id":"export-idem"}`)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()
	t.Setenv("NFS_GATEWAY_ENDPOINT", srv.URL)

	dr := newDR("nfs-idem", "tenant-nfs-idem", "nfs")
	r, ctx := reconcilerFor(t, dr)

	for i := 0; i < 3; i++ {
		if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
			t.Fatalf("Reconcile() iteration %d error = %v", i, err)
		}
		resetPhase(t, r, ctx, dr)
	}

	if got := atomic.LoadInt32(&posts); got != 1 {
		t.Errorf("gateway received %d POSTs across 3 reconciles, want 1 (duplicate exports stranded)", got)
	}
}
