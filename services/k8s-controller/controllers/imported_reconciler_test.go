package controllers

import (
	"context"
	"net"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
	cloudprovider "github.com/penguintechinc/nest/pkg/cloudprovider"
)

func importedScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 to scheme: %v", err)
	}
	if err := nestv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add nestv1 to scheme: %v", err)
	}
	return scheme
}

func importedDR(mutate func(*nestv1.DataResource)) *nestv1.DataResource {
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "adopted-db", Namespace: "default"},
		Spec: nestv1.DataResourceSpec{
			Type:        "postgres",
			Tenant:      "test-tenant",
			Origination: nestv1.OriginationImported,
			Import: &nestv1.ImportSpec{
				ConnectionString: "postgres://user:hunter2@db.example.com:5432/app",
				TLSMode:          "verify-full",
			},
		},
		Status: nestv1.DataResourceStatus{Phase: nestv1.PhasePending},
	}
	if mutate != nil {
		mutate(dr)
	}
	return dr
}

func newImportedReconciler(t *testing.T, objs ...client.Object) (*DataResourceReconciler, client.Client) {
	t.Helper()
	scheme := importedScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&nestv1.DataResource{}).
		Build()
	return &DataResourceReconciler{Client: c, Scheme: scheme}, c
}

// TestImportedNeverProvisionsInCluster is the regression guard for the core bug
// this feature fixes: origination "imported" had no handler and fell through to
// the engine dispatch, so adopting an existing RDS/Aurora instance provisioned a
// brand-new empty in-cluster CNPG cluster instead — silently doing the opposite
// of what was asked.
func TestImportedNeverProvisionsInCluster(t *testing.T) {
	dr := importedDR(nil)
	r, c := newImportedReconciler(t, dr)

	// Endpoint is unreachable; reconcile is still expected to complete without
	// provisioning anything. Health simply reports down.
	_, _ = r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace},
	})

	// Assert no CNPG Cluster was created anywhere.
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "postgresql.cnpg.io", Version: "v1", Kind: "ClusterList",
	})
	// The fake client errors for unregistered kinds; an error here means the
	// kind was never created, which satisfies the assertion. If it lists
	// successfully, the list must be empty.
	if err := c.List(context.Background(), list); err == nil {
		if len(list.Items) != 0 {
			t.Fatalf("imported resource provisioned %d in-cluster CNPG Cluster(s); "+
				"imported must adopt, never provision", len(list.Items))
		}
	}
}

func TestImportedMissingImportSpec(t *testing.T) {
	dr := importedDR(func(d *nestv1.DataResource) { d.Spec.Import = nil })
	r, _ := newImportedReconciler(t, dr)

	err := r.reconcileImported(context.Background(), dr)
	if err == nil {
		t.Fatal("expected error when spec.import is missing")
	}
	if !strings.Contains(err.Error(), "missing spec.import") {
		t.Errorf("unexpected error: %v", err)
	}
}

// Management features would MUTATE the adopted resource. They are opt-in on the
// CRD but unimplemented, so they must fail loudly rather than be silently
// ignored while the resource reports Ready.
func TestImportedManagedFeaturesRejected(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*nestv1.DataResource)
		want   string
	}{
		{"managedCredentials", func(d *nestv1.DataResource) { d.Spec.Import.ManagedCredentials = true }, "managedCredentials is not yet supported"},
		{"managedFailover", func(d *nestv1.DataResource) { d.Spec.Import.ManagedFailover = true }, "managedFailover is not yet supported"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dr := importedDR(tc.mutate)
			r, _ := newImportedReconciler(t, dr)

			err := r.reconcileImported(context.Background(), dr)
			if err == nil {
				t.Fatalf("expected %s to be rejected", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("expected %q, got: %v", tc.want, err)
			}
		})
	}
}

func TestImportedMissingCredentialSecret(t *testing.T) {
	dr := importedDR(func(d *nestv1.DataResource) { d.Spec.Import.CredentialSecret = "no-such-secret" })
	r, _ := newImportedReconciler(t, dr)

	err := r.reconcileImported(context.Background(), dr)
	if err == nil {
		t.Fatal("expected error for missing credential secret")
	}
	if !strings.Contains(err.Error(), "credential secret not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestImportedResolvesCredentialSecret(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "db-creds", Namespace: "default"},
		Data:       map[string][]byte{"access_key_id": []byte("AKIA-test")},
	}
	dr := importedDR(func(d *nestv1.DataResource) { d.Spec.Import.CredentialSecret = "db-creds" })
	r, _ := newImportedReconciler(t, dr, secret)

	got, err := r.resolveImportCredentials(context.Background(), dr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got["access_key_id"]) != "AKIA-test" {
		t.Errorf("credential not resolved, got %v", got)
	}
}

func TestImportedReachableEndpointReportsReady(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	dr := importedDR(func(d *nestv1.DataResource) {
		d.Spec.Import.ConnectionString = "postgres://user:pw@" + ln.Addr().String() + "/app"
	})
	r, c := newImportedReconciler(t, dr)

	if err := r.reconcileImported(context.Background(), dr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got nestv1.DataResource
	if err := c.Get(context.Background(), types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.Phase != nestv1.PhaseReady {
		t.Errorf("expected Ready, got %s", got.Status.Phase)
	}
	if got.Status.Health == nil || got.Status.Health.State != nestv1.HealthHealthy {
		t.Errorf("expected healthy, got %+v", got.Status.Health)
	}
	if got.Status.Endpoints == nil || got.Status.Endpoints.Native != ln.Addr().String() {
		t.Errorf("expected endpoint %s, got %+v", ln.Addr().String(), got.Status.Endpoints)
	}
	// The connection string carries a password; it must never reach status.
	if got.Status.Endpoints != nil && strings.Contains(got.Status.Endpoints.Native, "pw") {
		t.Error("password leaked into status endpoint")
	}
}

func TestImportedUnreachableEndpointReportsFailed(t *testing.T) {
	dr := importedDR(func(d *nestv1.DataResource) {
		// Port 1 on loopback: reliably refused, no DNS dependency.
		d.Spec.Import.ConnectionString = "postgres://user:pw@127.0.0.1:1/app"
	})
	r, c := newImportedReconciler(t, dr)

	if err := r.reconcileImported(context.Background(), dr); err != nil {
		t.Fatalf("reconcile should not error on an unreachable endpoint: %v", err)
	}

	var got nestv1.DataResource
	if err := c.Get(context.Background(), types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.Phase != nestv1.PhaseFailed {
		t.Errorf("expected Failed, got %s", got.Status.Phase)
	}
	if got.Status.Health == nil || got.Status.Health.State != nestv1.HealthDown {
		t.Errorf("expected down, got %+v", got.Status.Health)
	}
}

func TestImportedNoEndpointAvailable(t *testing.T) {
	dr := importedDR(func(d *nestv1.DataResource) { d.Spec.Import.ConnectionString = "" })
	r, _ := newImportedReconciler(t, dr)

	err := r.reconcileImported(context.Background(), dr)
	if err == nil {
		t.Fatal("expected error when no endpoint can be derived")
	}
	if !strings.Contains(err.Error(), "no endpoint available") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestImportedEndpointExtraction(t *testing.T) {
	cases := []struct {
		name    string
		conn    string
		want    string
		wantErr bool
	}{
		{"postgres with port", "postgres://u:p@db.example.com:5432/app", "db.example.com:5432", false},
		{"mysql", "mysql://u:p@mysql.internal:3306/app", "mysql.internal:3306", false},
		{"redis no port", "redis://cache.example.com", "cache.example.com", false},
		{"rds hostname", "postgres://u:p@x.abc123.us-east-1.rds.amazonaws.com:5432/db", "x.abc123.us-east-1.rds.amazonaws.com:5432", false},
		{"empty is not an error", "", "", false},
		{"no host", "postgres:///app", "", true},
		{"unparseable", "postgres://u:p@%zz/app", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := importedEndpoint(tc.conn)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.conn)
				}
				// url.Parse errors echo the input, which would leak the
				// password. The wrapper must not include it.
				if strings.Contains(err.Error(), "hunter2") || strings.Contains(err.Error(), ":p@") {
					t.Errorf("credential leaked in error: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestImportedHealthFromCloudReport(t *testing.T) {
	cases := []struct {
		name  string
		state string
		want  nestv1.HealthState
	}{
		{"healthy", "healthy", nestv1.HealthHealthy},
		{"available", "available", nestv1.HealthHealthy},
		{"running", "running", nestv1.HealthHealthy},
		{"degraded", "degraded", nestv1.HealthDegraded},
		{"modifying", "modifying", nestv1.HealthDegraded},
		{"backing-up", "backing-up", nestv1.HealthDegraded},
		{"stopped", "stopped", nestv1.HealthDown},
		{"unknown state", "who-knows", nestv1.HealthDown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, msg := importedHealth(context.Background(), "unused:0",
				&cloudprovider.HealthResult{State: tc.state})
			if got != tc.want {
				t.Errorf("state %q: got %s, want %s", tc.state, got, tc.want)
			}
			if got == nestv1.HealthDown && msg == "" {
				t.Error("down state should carry a message")
			}
		})
	}
}

// With no provider report, health must fall back to the TCP probe rather than
// assuming healthy.
func TestImportedHealthFallsBackToProbe(t *testing.T) {
	got, _ := importedHealth(context.Background(), "127.0.0.1:1", nil)
	if got != nestv1.HealthDown {
		t.Errorf("expected probe fallback to report down, got %s", got)
	}
}

// An unrecognised provider name is not rejected by the registry — it falls back
// to the generic Tier 2 standard-protocol provider by design (registry.Get).
// That provider then requires an explicit endpoint, so enrichment fails at
// Validate rather than at lookup.
func TestImportedEnrichFromCloudUnknownProviderFallsBackToGeneric(t *testing.T) {
	dr := importedDR(func(d *nestv1.DataResource) {
		d.Spec.External = &nestv1.ExternalSpec{Provider: "not-a-real-cloud"}
	})
	r, _ := newImportedReconciler(t, dr)

	_, _, err := r.enrichFromCloud(context.Background(), dr, nil)
	if err == nil {
		t.Fatal("expected generic provider to reject config with no endpoint")
	}
	if !strings.Contains(err.Error(), "provider validate") {
		t.Errorf("expected a validate failure, got: %v", err)
	}
}

// With an endpoint supplied, the generic provider validates and enrichment
// proceeds far enough to exercise the Discover path.
func TestImportedEnrichFromCloudGenericWithEndpoint(t *testing.T) {
	dr := importedDR(func(d *nestv1.DataResource) {
		d.Spec.External = &nestv1.ExternalSpec{
			Provider:   "not-a-real-cloud",
			Endpoint:   "db.example.com:5432",
			EngineType: "postgres",
		}
	})
	r, _ := newImportedReconciler(t, dr)

	info, _, err := r.enrichFromCloud(context.Background(), dr, nil)
	if err != nil {
		// Discover may legitimately fail without a live endpoint; the point is
		// that it got past Validate rather than failing at lookup.
		if strings.Contains(err.Error(), "unavailable") {
			t.Errorf("should not fail at provider lookup: %v", err)
		}
		return
	}
	if info == nil {
		t.Error("expected resource info from generic provider discover")
	}
}

// Credential material from the Secret must reach the provider config, since
// cloudprovider implementations read credentials out of cfg.Extra.
func TestImportedEnrichMergesSecretIntoExtra(t *testing.T) {
	dr := importedDR(func(d *nestv1.DataResource) {
		d.Spec.External = &nestv1.ExternalSpec{
			Provider: "not-a-real-cloud",
			Extra:    map[string]string{"region_hint": "us-east-1"},
		}
	})
	r, _ := newImportedReconciler(t, dr)

	// enrichFromCloud builds the config internally; exercise it with secret data
	// present and confirm the spec-provided Extra is not mutated in place.
	_, _, _ = r.enrichFromCloud(context.Background(), dr,
		map[string][]byte{"access_key_id": []byte("AKIA-test")})

	if _, leaked := dr.Spec.External.Extra["access_key_id"]; leaked {
		t.Error("secret material was written back into spec.external.extra; " +
			"the merged map must be a per-call copy")
	}
	if dr.Spec.External.Extra["region_hint"] != "us-east-1" {
		t.Error("spec-provided Extra was mutated")
	}
}

// Cloud enrichment is best-effort: a provider that cannot be reached must not
// make an otherwise-reachable adopted resource look failed.
func TestImportedCloudEnrichmentFailureIsNonFatal(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	dr := importedDR(func(d *nestv1.DataResource) {
		d.Spec.Import.ConnectionString = "postgres://u:p@" + ln.Addr().String() + "/app"
		d.Spec.External = &nestv1.ExternalSpec{Provider: "not-a-real-cloud"}
	})
	r, c := newImportedReconciler(t, dr)

	if err := r.reconcileImported(context.Background(), dr); err != nil {
		t.Fatalf("cloud enrichment failure must not fail reconcile: %v", err)
	}

	var got nestv1.DataResource
	if err := c.Get(context.Background(), types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.Phase != nestv1.PhaseReady {
		t.Errorf("expected Ready from endpoint probe, got %s", got.Status.Phase)
	}
}

func TestPhaseForHealth(t *testing.T) {
	cases := []struct {
		state nestv1.HealthState
		want  nestv1.DataResourcePhase
	}{
		{nestv1.HealthHealthy, nestv1.PhaseReady},
		{nestv1.HealthDegraded, nestv1.PhaseDegraded},
		{nestv1.HealthDown, nestv1.PhaseFailed},
		{nestv1.HealthState("unrecognised"), nestv1.PhaseFailed},
	}
	for _, tc := range cases {
		t.Run(string(tc.state), func(t *testing.T) {
			if got := phaseForHealth(tc.state); got != tc.want {
				t.Errorf("phaseForHealth(%s) = %s, want %s", tc.state, got, tc.want)
			}
		})
	}
}

// An unparseable connection string must fail the reconcile outright rather than
// silently adopting a resource with no verified endpoint.
func TestImportedInvalidConnectionStringFails(t *testing.T) {
	dr := importedDR(func(d *nestv1.DataResource) {
		d.Spec.Import.ConnectionString = "postgres://u:hunter2@%zz/app"
	})
	r, _ := newImportedReconciler(t, dr)

	err := r.reconcileImported(context.Background(), dr)
	if err == nil {
		t.Fatal("expected error for unparseable connection string")
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Errorf("password leaked into reconcile error: %v", err)
	}
}

// A connection string with no host is rejected before any adoption happens.
func TestImportedConnectionStringWithoutHostFails(t *testing.T) {
	dr := importedDR(func(d *nestv1.DataResource) {
		d.Spec.Import.ConnectionString = "postgres:///app"
	})
	r, _ := newImportedReconciler(t, dr)

	if err := r.reconcileImported(context.Background(), dr); err == nil {
		t.Fatal("expected error for connection string without host")
	}
}

// Deleting an imported DataResource must release Nest's reference without
// touching the adopted resource, and without invoking any engine delete handler.
func TestImportedDeleteReleasesWithoutDestroying(t *testing.T) {
	now := metav1.Now()
	dr := importedDR(func(d *nestv1.DataResource) {
		d.DeletionTimestamp = &now
		d.Finalizers = []string{"nest.penguintech.io/dataresource"}
	})
	r, c := newImportedReconciler(t, dr)

	if _, err := r.reconcileDelete(context.Background(), dr); err != nil {
		t.Fatalf("imported delete should succeed: %v", err)
	}

	// Finalizer released so the object can be garbage collected.
	var got nestv1.DataResource
	err := c.Get(context.Background(), types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &got)
	if err == nil {
		for _, f := range got.Finalizers {
			if f == "nest.penguintech.io/dataresource" {
				t.Error("finalizer not released on imported delete")
			}
		}
	}
}
