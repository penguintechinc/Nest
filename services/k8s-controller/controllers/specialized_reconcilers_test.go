package controllers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func newDR(name, tenant, drType string) *nestv1.DataResource {
	return &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: nestv1.DataResourceSpec{
			Type:   drType,
			Tenant: tenant,
		},
	}
}

func reqFor(dr *nestv1.DataResource) ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}}
}

func reconcilerFor(t *testing.T, objects ...interface{}) (*DataResourceReconciler, context.Context) {
	t.Helper()
	scheme := newTestScheme(t)
	builder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&nestv1.DataResource{})
	for _, raw := range objects {
		switch v := raw.(type) {
		case *nestv1.DataResource:
			builder = builder.WithObjects(v)
		case *nestv1.HardwareInventory:
			builder = builder.WithObjects(v)
		case *corev1.ConfigMap:
			builder = builder.WithObjects(v)
		case *corev1.Service:
			builder = builder.WithObjects(v)
		case *corev1.PersistentVolumeClaim:
			builder = builder.WithObjects(v)
		case *corev1.Secret:
			builder = builder.WithObjects(v)
		case *appsv1.Deployment:
			builder = builder.WithObjects(v)
		case *appsv1.StatefulSet:
			builder = builder.WithObjects(v)
		}
	}
	fakeClient := builder.Build()
	r := &DataResourceReconciler{Client: fakeClient, Scheme: scheme}
	return r, context.Background()
}

// ─────────────────────────────────────────────────────────────────────────────
// Clickhouse Reconciler
// ─────────────────────────────────────────────────────────────────────────────

func TestClickhouse_Reconcile_NewResource(t *testing.T) {
	dr := newDR("ch-resource", "tenant-ch", "clickhouse")
	r, ctx := reconcilerFor(t, dr)
	_, err := r.Reconcile(ctx, reqFor(dr))
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	var updated nestv1.DataResource
	if err := r.Client.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if updated.Status.Phase != nestv1.PhaseProvisioning {
		t.Errorf("Status.Phase = %v, want Provisioning", updated.Status.Phase)
	}
}

func TestClickhouse_Reconcile_IdempotentSecondCall(t *testing.T) {
	dr := newDR("ch-idem", "tenant-ch2", "clickhouse")
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
}

func TestClickhouse_Reconcile_NotFound(t *testing.T) {
	r, ctx := reconcilerFor(t)
	dr := newDR("missing-ch", "tenant-ch3", "clickhouse")
	_, err := r.Reconcile(ctx, reqFor(dr))
	if err != nil {
		t.Fatalf("Reconcile() on missing resource error = %v", err)
	}
}

func TestClickhouse_Reconcile_WithStorageSize(t *testing.T) {
	dr := newDR("ch-storage", "tenant-ch4", "clickhouse")
	dr.Spec.Size = &nestv1.ResourceSize{Storage: "200Gi"}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestClickhouse_HelperFunctions(t *testing.T) {
	dr := newDR("mych", "mytenant", "clickhouse")
	if got := clickhouseClusterName(dr); got != "mytenant-mych" {
		t.Errorf("clickhouseClusterName = %q, want %q", got, "mytenant-mych")
	}
	if got := clickhouseNamespace(dr); got != "mytenant" {
		t.Errorf("clickhouseNamespace = %q, want %q", got, "mytenant")
	}
	if got := clickhouseStorageSize(dr); got != "10Gi" {
		t.Errorf("clickhouseStorageSize (default) = %q, want 10Gi", got)
	}
	dr.Spec.Size = &nestv1.ResourceSize{Storage: "50Gi"}
	if got := clickhouseStorageSize(dr); got != "50Gi" {
		t.Errorf("clickhouseStorageSize (custom) = %q, want 50Gi", got)
	}
	// Invalid quantity falls back to default
	dr.Spec.Size = &nestv1.ResourceSize{Storage: "invalid"}
	if got := clickhouseStorageSize(dr); got != "10Gi" {
		t.Errorf("clickhouseStorageSize (invalid) = %q, want 10Gi", got)
	}
}

func TestClickhouse_Delete(t *testing.T) {
	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "ch-del",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{Type: "clickhouse", Tenant: "tenant-ch-del"},
	}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Kafka Reconciler
// ─────────────────────────────────────────────────────────────────────────────

func TestKafka_Reconcile_NewResource(t *testing.T) {
	dr := newDR("kafka-resource", "tenant-kafka", "kafka")
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	var updated nestv1.DataResource
	if err := r.Client.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if updated.Status.Phase != nestv1.PhaseProvisioning {
		t.Errorf("Status.Phase = %v, want Provisioning", updated.Status.Phase)
	}
}

func TestKafka_Reconcile_NotFound(t *testing.T) {
	r, ctx := reconcilerFor(t)
	dr := newDR("missing-kafka", "tenant-kafka2", "kafka")
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() on missing resource error = %v", err)
	}
}

func TestKafka_Reconcile_WithReplicas(t *testing.T) {
	dr := newDR("kafka-replicas", "tenant-kafka3", "kafka")
	dr.Spec.Replicas = &nestv1.ReplicaConfig{
		Write: &nestv1.ReplicaCountSpec{Default: 5},
	}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestKafka_HelperFunctions(t *testing.T) {
	dr := newDR("mykafka", "mytenantk", "kafka")
	if got := kafkaClusterName(dr); got != "mytenantk-mykafka" {
		t.Errorf("kafkaClusterName = %q", got)
	}
	if got := kafkaNamespace(dr); got != "mytenantk" {
		t.Errorf("kafkaNamespace = %q", got)
	}
	if got := kafkaStorageSize(dr); got != "10Gi" {
		t.Errorf("kafkaStorageSize default = %q, want 10Gi", got)
	}
	dr.Spec.Size = &nestv1.ResourceSize{Storage: "100Gi"}
	if got := kafkaStorageSize(dr); got != "100Gi" {
		t.Errorf("kafkaStorageSize custom = %q, want 100Gi", got)
	}
}

func TestKafka_Delete(t *testing.T) {
	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "kafka-del",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{Type: "kafka", Tenant: "tenant-kafka-del"},
	}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// MariaDB Reconciler
// ─────────────────────────────────────────────────────────────────────────────

func TestMariaDB_Reconcile_NewResource(t *testing.T) {
	dr := newDR("mariadb-resource", "tenant-mdb", "mariadb")
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	var updated nestv1.DataResource
	if err := r.Client.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if updated.Status.Phase != nestv1.PhaseProvisioning {
		t.Errorf("Status.Phase = %v, want Provisioning", updated.Status.Phase)
	}
}

func TestMariaDB_Reconcile_NotFound(t *testing.T) {
	r, ctx := reconcilerFor(t)
	dr := newDR("missing-mdb", "tenant-mdb2", "mariadb")
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestMariaDB_Reconcile_WithReplicas(t *testing.T) {
	dr := newDR("mariadb-replicas", "tenant-mdb3", "mariadb")
	dr.Spec.Replicas = &nestv1.ReplicaConfig{
		Write: &nestv1.ReplicaCountSpec{Min: 5},
	}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestMariaDB_HelperFunctions(t *testing.T) {
	dr := newDR("mymdb", "mytenantmdb", "mariadb")
	if got := mariadbClusterName(dr); got != "mytenantmdb-mymdb-galera" {
		t.Errorf("mariadbClusterName = %q", got)
	}
	if got := mariadbStorageSize(dr); got != "20Gi" {
		t.Errorf("mariadbStorageSize default = %q, want 20Gi", got)
	}
	dr.Spec.Size = &nestv1.ResourceSize{Storage: "250Gi"}
	if got := mariadbStorageSize(dr); got != "250Gi" {
		t.Errorf("mariadbStorageSize custom = %q, want 250Gi", got)
	}
}

func TestMariaDB_Delete(t *testing.T) {
	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "mdb-del",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{Type: "mariadb", Tenant: "tenant-mdb-del"},
	}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// MySQL Reconciler
// ─────────────────────────────────────────────────────────────────────────────

func TestMySQL_Reconcile_NewResource(t *testing.T) {
	dr := newDR("mysql-resource", "tenant-mysql", "mysql")
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	var updated nestv1.DataResource
	if err := r.Client.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if updated.Status.Phase != nestv1.PhaseProvisioning {
		t.Errorf("Status.Phase = %v, want Provisioning", updated.Status.Phase)
	}
}

func TestMySQL_Reconcile_NotFound(t *testing.T) {
	r, ctx := reconcilerFor(t)
	dr := newDR("missing-mysql", "tenant-mysql2", "mysql")
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestMySQL_Reconcile_WithReplicas(t *testing.T) {
	dr := newDR("mysql-replicas", "tenant-mysql3", "mysql")
	dr.Spec.Replicas = &nestv1.ReplicaConfig{
		Write: &nestv1.ReplicaCountSpec{Min: 3},
		Read:  &nestv1.ReplicaCountSpec{Default: 2},
	}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestMySQL_HelperFunctions(t *testing.T) {
	dr := newDR("mymysql", "mytenantmysql", "mysql")
	if got := mysqlClusterName(dr); got != "mytenantmysql-mymysql-mysql" {
		t.Errorf("mysqlClusterName = %q", got)
	}
	if got := mysqlStorageSize(dr); got != "20Gi" {
		t.Errorf("mysqlStorageSize default = %q, want 20Gi", got)
	}
	dr.Spec.Size = &nestv1.ResourceSize{Storage: "300Gi"}
	if got := mysqlStorageSize(dr); got != "300Gi" {
		t.Errorf("mysqlStorageSize custom = %q, want 300Gi", got)
	}
}

func TestMySQL_Delete(t *testing.T) {
	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "mysql-del",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{Type: "mysql", Tenant: "tenant-mysql-del"},
	}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// OpenSearch Reconciler
// ─────────────────────────────────────────────────────────────────────────────

func TestOpenSearch_Reconcile_NewResource(t *testing.T) {
	dr := newDR("opensearch-resource", "tenant-os", "search")
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	var updated nestv1.DataResource
	if err := r.Client.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if updated.Status.Phase != nestv1.PhaseProvisioning {
		t.Errorf("Status.Phase = %v, want Provisioning", updated.Status.Phase)
	}
}

func TestOpenSearch_Reconcile_NotFound(t *testing.T) {
	r, ctx := reconcilerFor(t)
	dr := newDR("missing-os", "tenant-os2", "search")
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestOpenSearch_HelperFunctions(t *testing.T) {
	dr := newDR("myos", "mytenantos", "search")
	if got := opensearchClusterName(dr); got != "mytenantos-myos-opensearch" {
		t.Errorf("opensearchClusterName = %q", got)
	}
	if got := opensearchNamespace(dr); got != "mytenantos" {
		t.Errorf("opensearchNamespace = %q", got)
	}
	if got := opensearchStorageSize(dr); got != "30Gi" {
		t.Errorf("opensearchStorageSize default = %q, want 30Gi", got)
	}
	dr.Spec.Size = &nestv1.ResourceSize{Storage: "500Gi"}
	if got := opensearchStorageSize(dr); got != "500Gi" {
		t.Errorf("opensearchStorageSize custom = %q, want 500Gi", got)
	}
}

func TestOpenSearch_Delete(t *testing.T) {
	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "os-del",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{Type: "search", Tenant: "tenant-os-del"},
	}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Timeseries Reconciler
// ─────────────────────────────────────────────────────────────────────────────

func TestTimeseries_Reconcile_NewResource(t *testing.T) {
	dr := newDR("ts-resource", "tenant-ts", "timeseries")
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	var updated nestv1.DataResource
	if err := r.Client.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if updated.Status.Phase != nestv1.PhaseProvisioning {
		t.Errorf("Status.Phase = %v, want Provisioning", updated.Status.Phase)
	}
}

func TestTimeseries_Reconcile_NotFound(t *testing.T) {
	r, ctx := reconcilerFor(t)
	dr := newDR("missing-ts", "tenant-ts2", "timeseries")
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestTimeseries_Reconcile_Idempotent(t *testing.T) {
	dr := newDR("ts-idem", "tenant-ts3", "timeseries")
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
}

func TestTimeseries_HelperFunctions(t *testing.T) {
	dr := newDR("myts", "mytenantts", "timeseries")
	if got := timeseriesServiceName(dr); got != "mytenantts-myts-vm" {
		t.Errorf("timeseriesServiceName = %q", got)
	}
	if got := timeseriesStatefulSetName(dr); got != "mytenantts-myts-vm" {
		t.Errorf("timeseriesStatefulSetName = %q", got)
	}
	if got := timeseriesStorageSize(dr); got != "50Gi" {
		t.Errorf("timeseriesStorageSize default = %q, want 50Gi", got)
	}
	dr.Spec.Size = &nestv1.ResourceSize{Storage: "1Ti"}
	if got := timeseriesStorageSize(dr); got != "1Ti" {
		t.Errorf("timeseriesStorageSize custom = %q, want 1Ti", got)
	}
}

func TestTimeseries_Delete(t *testing.T) {
	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "ts-del",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{Type: "timeseries", Tenant: "tenant-ts-del"},
	}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Vector Reconciler
// ─────────────────────────────────────────────────────────────────────────────

func TestVector_Reconcile_NewResource(t *testing.T) {
	dr := newDR("vec-resource", "tenant-vec", "vector")
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	var updated nestv1.DataResource
	if err := r.Client.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if updated.Status.Phase != nestv1.PhaseProvisioning {
		t.Errorf("Status.Phase = %v, want Provisioning", updated.Status.Phase)
	}
}

func TestVector_Reconcile_NotFound(t *testing.T) {
	r, ctx := reconcilerFor(t)
	dr := newDR("missing-vec", "tenant-vec2", "vector")
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestVector_Reconcile_WithReplicas(t *testing.T) {
	dr := newDR("vec-replicas", "tenant-vec3", "vector")
	dr.Spec.Replicas = &nestv1.ReplicaConfig{
		Write: &nestv1.ReplicaCountSpec{Min: 2},
		Read:  &nestv1.ReplicaCountSpec{Default: 3},
	}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestVector_HelperFunctions(t *testing.T) {
	dr := newDR("myvec", "mytenantvec", "vector")
	if got := vectorClusterName(dr); got != "mytenantvec-myvec-vector" {
		t.Errorf("vectorClusterName = %q", got)
	}
	if got := vectorNamespace(dr); got != "mytenantvec" {
		t.Errorf("vectorNamespace = %q", got)
	}
	if got := vectorStorageSize(dr); got != "10Gi" {
		t.Errorf("vectorStorageSize default = %q, want 10Gi", got)
	}
	dr.Spec.Size = &nestv1.ResourceSize{Storage: "50Gi"}
	if got := vectorStorageSize(dr); got != "50Gi" {
		t.Errorf("vectorStorageSize custom = %q, want 50Gi", got)
	}
}

func TestVector_Delete(t *testing.T) {
	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "vec-del",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{Type: "vector", Tenant: "tenant-vec-del"},
	}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// FerretDB Reconciler
// ─────────────────────────────────────────────────────────────────────────────

func TestFerretDB_Reconcile_NewResource(t *testing.T) {
	dr := newDR("ferretdb-resource", "tenant-fdb", "rockfs")
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	var updated nestv1.DataResource
	if err := r.Client.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if updated.Status.Phase != nestv1.PhaseProvisioning {
		t.Errorf("Status.Phase = %v, want Provisioning", updated.Status.Phase)
	}
}

func TestFerretDB_Reconcile_NotFound(t *testing.T) {
	r, ctx := reconcilerFor(t)
	dr := newDR("missing-fdb", "tenant-fdb2", "rockfs")
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestFerretDB_Reconcile_Idempotent(t *testing.T) {
	dr := newDR("fdb-idem", "tenant-fdb3", "rockfs")
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
}

func TestFerretDB_HelperFunctions(t *testing.T) {
	dr := newDR("myfdb", "mytenantfdb", "rockfs")
	if got := ferretdbPGClusterName(dr); got != "mytenantfdb-myfdb-ferretdb-pg" {
		t.Errorf("ferretdbPGClusterName = %q", got)
	}
	if got := ferretdbDeploymentName(dr); got != "mytenantfdb-myfdb-ferretdb" {
		t.Errorf("ferretdbDeploymentName = %q", got)
	}
	if got := ferretdbNamespace(dr); got != "mytenantfdb" {
		t.Errorf("ferretdbNamespace = %q", got)
	}
	if got := ferretdbStorageSize(dr); got != "10Gi" {
		t.Errorf("ferretdbStorageSize default = %q, want 10Gi", got)
	}
	dr.Spec.Size = &nestv1.ResourceSize{Storage: "200Gi"}
	if got := ferretdbStorageSize(dr); got != "200Gi" {
		t.Errorf("ferretdbStorageSize custom = %q, want 200Gi", got)
	}
}

func TestFerretDB_Delete(t *testing.T) {
	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "fdb-del",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{Type: "rockfs", Tenant: "tenant-fdb-del"},
	}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Filesystem Reconciler
// ─────────────────────────────────────────────────────────────────────────────

func TestFilesystem_Reconcile_NewResource(t *testing.T) {
	dr := newDR("fs-resource", "tenant-fs", "filesystem")
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	var updated nestv1.DataResource
	if err := r.Client.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if updated.Status.Phase != nestv1.PhaseProvisioning {
		t.Errorf("Status.Phase = %v, want Provisioning", updated.Status.Phase)
	}
}

func TestFilesystem_Reconcile_NotFound(t *testing.T) {
	r, ctx := reconcilerFor(t)
	dr := newDR("missing-fs", "tenant-fs2", "filesystem")
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestFilesystem_Reconcile_WithAnnotation(t *testing.T) {
	dr := newDR("fs-ann", "tenant-fs3", "filesystem")
	dr.Annotations = map[string]string{
		"nest.penguintech.io/storage-class": "rook-cephfs-fast",
	}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestFilesystem_Reconcile_Idempotent(t *testing.T) {
	dr := newDR("fs-idem", "tenant-fs4", "filesystem")
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
}

func TestFilesystem_HelperFunctions(t *testing.T) {
	dr := newDR("myfs", "mytenantfs", "filesystem")
	if got := filesystemPVCName(dr); got != "mytenantfs-myfs-cephfs" {
		t.Errorf("filesystemPVCName = %q", got)
	}
	if got := filesystemStorageSize(dr); got != "10Gi" {
		t.Errorf("filesystemStorageSize default = %q, want 10Gi", got)
	}
	dr.Spec.Size = &nestv1.ResourceSize{Storage: "1Ti"}
	if got := filesystemStorageSize(dr); got != "1Ti" {
		t.Errorf("filesystemStorageSize custom = %q, want 1Ti", got)
	}
}

func TestFilesystem_Delete(t *testing.T) {
	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "fs-del",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{Type: "filesystem", Tenant: "tenant-fs-del"},
	}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Iceberg Reconciler
// ─────────────────────────────────────────────────────────────────────────────

func TestIceberg_Reconcile_NewResource(t *testing.T) {
	dr := newDR("iceberg-resource", "tenant-ice", "lakehouse/iceberg")
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	var updated nestv1.DataResource
	if err := r.Client.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if updated.Status.Phase != nestv1.PhaseProvisioning {
		t.Errorf("Status.Phase = %v, want Provisioning", updated.Status.Phase)
	}
}

func TestIceberg_Reconcile_NotFound(t *testing.T) {
	r, ctx := reconcilerFor(t)
	dr := newDR("missing-ice", "tenant-ice2", "lakehouse/iceberg")
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestIceberg_Reconcile_Idempotent(t *testing.T) {
	dr := newDR("ice-idem", "tenant-ice3", "lakehouse/iceberg")
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
}

func TestIceberg_HelperFunctions(t *testing.T) {
	dr := newDR("myice", "mytenantice", "lakehouse/iceberg")
	if got := icebergNamespace(dr); got != "mytenantice" {
		t.Errorf("icebergNamespace = %q", got)
	}
	if got := icebergRestName(dr); got != "mytenantice-myice-iceberg-rest" {
		t.Errorf("icebergRestName = %q", got)
	}
	if got := icebergPGClusterName(dr); got != "mytenantice-myice-iceberg-pg" {
		t.Errorf("icebergPGClusterName = %q", got)
	}
}

func TestIceberg_Delete(t *testing.T) {
	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "ice-del",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{Type: "lakehouse/iceberg", Tenant: "tenant-ice-del"},
	}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Trino Reconciler
// ─────────────────────────────────────────────────────────────────────────────

func TestTrino_Reconcile_NewResource(t *testing.T) {
	dr := newDR("trino-resource", "tenant-trino", "warehouse/trino")
	dr.Spec.Annotations = map[string]string{}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	var updated nestv1.DataResource
	if err := r.Client.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if updated.Status.Phase != nestv1.PhaseProvisioning {
		t.Errorf("Status.Phase = %v, want Provisioning", updated.Status.Phase)
	}
}

func TestTrino_Reconcile_NotFound(t *testing.T) {
	r, ctx := reconcilerFor(t)
	dr := newDR("missing-trino", "tenant-trino2", "warehouse/trino")
	dr.Spec.Annotations = map[string]string{}
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestTrino_Reconcile_WithIcebergAnnotation(t *testing.T) {
	dr := newDR("trino-ice", "tenant-trino3", "warehouse/trino")
	dr.Spec.Annotations = map[string]string{
		"nest.penguintech.io/iceberg-endpoint": "iceberg-rest.tenant-trino3.svc.cluster.local:8181",
	}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestTrino_Reconcile_Idempotent(t *testing.T) {
	dr := newDR("trino-idem", "tenant-trino4", "warehouse/trino")
	dr.Spec.Annotations = map[string]string{}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
}

func TestTrino_HelperFunctions(t *testing.T) {
	dr := newDR("mytrino", "mytenanttrino", "warehouse/trino")
	if got := trinoNamespace(dr); got != "mytenanttrino" {
		t.Errorf("trinoNamespace = %q", got)
	}
	if got := trinoCoordinatorName(dr); got != "mytenanttrino-mytrino-trino-coordinator" {
		t.Errorf("trinoCoordinatorName = %q", got)
	}
	if got := trinoWorkerName(dr); got != "mytenanttrino-mytrino-trino-worker" {
		t.Errorf("trinoWorkerName = %q", got)
	}
	if got := trinoServiceName(dr); got != "mytenanttrino-mytrino-trino" {
		t.Errorf("trinoServiceName = %q", got)
	}
	if got := trinoCoordinatorConfigMapName(dr); got != "mytenanttrino-mytrino-trino-config" {
		t.Errorf("trinoCoordinatorConfigMapName = %q", got)
	}
	if got := trinoWorkerConfigMapName(dr); got != "mytenanttrino-mytrino-trino-worker-config" {
		t.Errorf("trinoWorkerConfigMapName = %q", got)
	}
}

func TestTrino_Delete(t *testing.T) {
	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "trino-del",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{
			Type:        "warehouse/trino",
			Tenant:      "tenant-trino-del",
			Annotations: map[string]string{},
		},
	}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Postgres Reconciler (additional coverage for uncovered helpers)
// ─────────────────────────────────────────────────────────────────────────────

func TestPostgres_HelperFunctions(t *testing.T) {
	dr := newDR("mypg", "mytenantpg", "postgres")
	if got := postgresClusterName(dr); got != "mytenantpg-mypg" {
		t.Errorf("postgresClusterName = %q", got)
	}
	if got := postgresNamespace(dr); got != "mytenantpg" {
		t.Errorf("postgresNamespace = %q", got)
	}
	if got := postgresStorageSize(dr); got != "10Gi" {
		t.Errorf("postgresStorageSize default = %q, want 10Gi", got)
	}
	dr.Spec.Size = &nestv1.ResourceSize{Storage: "500Gi"}
	if got := postgresStorageSize(dr); got != "500Gi" {
		t.Errorf("postgresStorageSize custom = %q, want 500Gi", got)
	}
}

func TestPostgres_EnsureNamespace(t *testing.T) {
	r, ctx := reconcilerFor(t)
	// First call creates namespace
	if err := r.ensurePostgresNamespace(ctx, "my-test-ns"); err != nil {
		t.Fatalf("ensurePostgresNamespace (create) error = %v", err)
	}
	// Second call should be idempotent (AlreadyExists → no error)
	if err := r.ensurePostgresNamespace(ctx, "my-test-ns"); err != nil {
		t.Fatalf("ensurePostgresNamespace (already exists) error = %v", err)
	}
}

func TestPostgres_DblbConfigMap(t *testing.T) {
	dr := newDR("pg-dblb", "tenant-dblb", "postgres")
	r, ctx := reconcilerFor(t, dr)

	// Create the namespace first (reconcileDblbConfig needs it)
	if err := r.ensurePostgresNamespace(ctx, dr.Spec.Tenant); err != nil {
		t.Fatalf("ensurePostgresNamespace error = %v", err)
	}

	// Create DBLB configmap
	if err := r.reconcileDblbConfig(ctx, dr); err != nil {
		t.Fatalf("reconcileDblbConfig (create) error = %v", err)
	}
	// Idempotent update
	if err := r.reconcileDblbConfig(ctx, dr); err != nil {
		t.Fatalf("reconcileDblbConfig (update) error = %v", err)
	}
}

func TestPostgres_DblbConfigMapName(t *testing.T) {
	dr := newDR("mypg", "mytenantpg", "postgres")
	if got := dblbConfigMapName(dr); got != "dblb-mytenantpg-mypg" {
		t.Errorf("dblbConfigMapName = %q", got)
	}
}

func TestPostgres_Reconcile_WithReplicas(t *testing.T) {
	dr := newDR("pg-replicas", "tenant-pgrep", "postgres")
	dr.Spec.Replicas = &nestv1.ReplicaConfig{
		Write: &nestv1.ReplicaCountSpec{Min: 3},
		Read:  &nestv1.ReplicaCountSpec{Default: 2},
	}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// NFS Reconciler (uses HTTP gateway — mock with httptest)
// ─────────────────────────────────────────────────────────────────────────────

func TestNFS_Reconcile_GatewaySuccess(t *testing.T) {
	// Mock NFS gateway that returns 201 with an id
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintln(w, `{"id":"export-001"}`)
	}))
	defer srv.Close()
	t.Setenv("NFS_GATEWAY_ENDPOINT", srv.URL)

	dr := newDR("nfs-resource", "tenant-nfs", "nfs")
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	var updated nestv1.DataResource
	if err := r.Client.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if updated.Status.Phase != nestv1.PhaseReady {
		t.Errorf("Status.Phase = %v, want Ready", updated.Status.Phase)
	}
}

func TestNFS_Reconcile_GatewayError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	t.Setenv("NFS_GATEWAY_ENDPOINT", srv.URL)

	dr := newDR("nfs-error", "tenant-nfs2", "nfs")
	r, ctx := reconcilerFor(t, dr)
	_, err := r.Reconcile(ctx, reqFor(dr))
	// Error is expected when gateway returns non-201
	if err == nil {
		t.Error("expected error from NFS gateway 500 response, got nil")
	}
}

func TestNFS_Reconcile_NotFound(t *testing.T) {
	r, ctx := reconcilerFor(t)
	t.Setenv("NFS_GATEWAY_ENDPOINT", "http://localhost:1")
	dr := newDR("missing-nfs", "tenant-nfs3", "nfs")
	// Missing resource should not error
	_, err := r.Reconcile(ctx, reqFor(dr))
	if err != nil {
		t.Fatalf("Reconcile() on missing resource error = %v", err)
	}
}

func TestNFS_Delete_NoAnnotations(t *testing.T) {
	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "nfs-del-noanno",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{Type: "nfs", Tenant: "tenant-nfs-del"},
	}
	r, ctx := reconcilerFor(t, dr)
	// Without export-id annotation, delete should skip gateway call
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete/no-annotation) error = %v", err)
	}
}

func TestNFS_Delete_WithAnnotationSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	t.Setenv("NFS_GATEWAY_ENDPOINT", srv.URL)

	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "nfs-del-ok",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
			Annotations: map[string]string{
				"nest.penguintech.io/nfs-export-id": "export-001",
			},
		},
		Spec: nestv1.DataResourceSpec{Type: "nfs", Tenant: "tenant-nfs-del2"},
	}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete/with-annotation) error = %v", err)
	}
}

func TestNFS_Reconcile_GatewayMissingIDField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintln(w, `{"result":"ok"}`) // no "id" field
	}))
	defer srv.Close()
	t.Setenv("NFS_GATEWAY_ENDPOINT", srv.URL)

	dr := newDR("nfs-noid", "tenant-nfs4", "nfs")
	r, ctx := reconcilerFor(t, dr)
	_, err := r.Reconcile(ctx, reqFor(dr))
	if err == nil {
		t.Error("expected error when NFS gateway response is missing 'id' field")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// iSCSI Reconciler (uses HTTP gateway — mock with httptest)
// ─────────────────────────────────────────────────────────────────────────────

func TestISCSI_Reconcile_GatewaySuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintln(w, `{"id":"target-001","iqn":"iqn.2024-01.io.penguintech:target-001"}`)
	}))
	defer srv.Close()
	t.Setenv("ISCSI_GATEWAY_ENDPOINT", srv.URL)

	dr := newDR("iscsi-resource", "tenant-iscsi", "iscsi")
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	var updated nestv1.DataResource
	if err := r.Client.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if updated.Status.Phase != nestv1.PhaseReady {
		t.Errorf("Status.Phase = %v, want Ready", updated.Status.Phase)
	}
}

func TestISCSI_Reconcile_GatewayError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	t.Setenv("ISCSI_GATEWAY_ENDPOINT", srv.URL)

	dr := newDR("iscsi-error", "tenant-iscsi2", "iscsi")
	r, ctx := reconcilerFor(t, dr)
	_, err := r.Reconcile(ctx, reqFor(dr))
	if err == nil {
		t.Error("expected error from iSCSI gateway 500 response, got nil")
	}
}

func TestISCSI_Reconcile_NotFound(t *testing.T) {
	r, ctx := reconcilerFor(t)
	dr := newDR("missing-iscsi", "tenant-iscsi3", "iscsi")
	// Missing resource should not error
	_, err := r.Reconcile(ctx, reqFor(dr))
	if err != nil {
		t.Fatalf("Reconcile() on missing resource error = %v", err)
	}
}

func TestISCSI_Reconcile_WithStorageSize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintln(w, `{"id":"target-002","iqn":"iqn.2024-01.io.penguintech:target-002"}`)
	}))
	defer srv.Close()
	t.Setenv("ISCSI_GATEWAY_ENDPOINT", srv.URL)

	dr := newDR("iscsi-size", "tenant-iscsi4", "iscsi")
	dr.Spec.Size = &nestv1.ResourceSize{Storage: "500Gi"}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestISCSI_Delete_NoAnnotations(t *testing.T) {
	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "iscsi-del-noanno",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{Type: "iscsi", Tenant: "tenant-iscsi-del"},
	}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete/no-annotation) error = %v", err)
	}
}

func TestISCSI_Delete_WithAnnotationSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	t.Setenv("ISCSI_GATEWAY_ENDPOINT", srv.URL)

	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "iscsi-del-ok",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
			Annotations: map[string]string{
				"nest.penguintech.io/iscsi-target-id": "target-001",
			},
		},
		Spec: nestv1.DataResourceSpec{Type: "iscsi", Tenant: "tenant-iscsi-del2"},
	}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete/with-annotation) error = %v", err)
	}
}

func TestISCSI_StorageSizeHelper(t *testing.T) {
	dr := newDR("myiscsi", "mytenantiscsi", "iscsi")
	if got := iscsiStorageSize(dr); got != "100Gi" {
		t.Errorf("iscsiStorageSize default = %q, want 100Gi", got)
	}
	dr.Spec.Size = &nestv1.ResourceSize{Storage: "2Ti"}
	if got := iscsiStorageSize(dr); got != "2Ti" {
		t.Errorf("iscsiStorageSize custom = %q, want 2Ti", got)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Placement Engine
// ─────────────────────────────────────────────────────────────────────────────

func newPlacementEngine(t *testing.T, objects ...interface{}) (*PlacementEngine, context.Context) {
	t.Helper()
	scheme := newTestScheme(t)
	builder := fake.NewClientBuilder().WithScheme(scheme)
	for _, raw := range objects {
		if inv, ok := raw.(*nestv1.HardwareInventory); ok {
			builder = builder.WithObjects(inv)
		}
	}
	fakeClient := builder.Build()
	return NewPlacementEngine(fakeClient), context.Background()
}

func TestPlacement_NodeAffinity_NoInventory(t *testing.T) {
	p, ctx := newPlacementEngine(t)
	affinity, err := p.NodeAffinity(ctx, "nvme")
	if err != nil {
		t.Fatalf("NodeAffinity() error = %v", err)
	}
	if affinity != nil {
		t.Errorf("expected nil affinity with no inventory, got %v", affinity)
	}
}

func TestPlacement_NodeAffinity_WithActiveDevices(t *testing.T) {
	inv := &nestv1.HardwareInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "node1-inv", Namespace: "default"},
		Spec: nestv1.HardwareInventorySpec{
			Node: "node-1",
			Devices: []nestv1.DeviceSpec{
				{Name: "/dev/nvme0n1", Class: "nvme", State: nestv1.DeviceStateActive, CapacityBytes: 1000000000},
				{Name: "/dev/sda", Class: "hdd", State: nestv1.DeviceStateActive, CapacityBytes: 4000000000},
			},
		},
	}
	p, ctx := newPlacementEngine(t, inv)
	affinity, err := p.NodeAffinity(ctx, "nvme")
	if err != nil {
		t.Fatalf("NodeAffinity() error = %v", err)
	}
	if affinity == nil {
		t.Fatal("expected non-nil affinity for node with active nvme devices")
	}
	terms := affinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	if len(terms) != 1 {
		t.Fatalf("expected 1 selector term, got %d", len(terms))
	}
	if len(terms[0].MatchExpressions) != 1 {
		t.Fatalf("expected 1 match expression, got %d", len(terms[0].MatchExpressions))
	}
	if terms[0].MatchExpressions[0].Values[0] != "node-1" {
		t.Errorf("expected node-1 in affinity values, got %v", terms[0].MatchExpressions[0].Values)
	}
}

func TestPlacement_NodeAffinity_NoActiveDevicesInClass(t *testing.T) {
	inv := &nestv1.HardwareInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "node1-inv2", Namespace: "default"},
		Spec: nestv1.HardwareInventorySpec{
			Node: "node-1",
			Devices: []nestv1.DeviceSpec{
				{Name: "/dev/nvme0n1", Class: "nvme", State: nestv1.DeviceStatePending, CapacityBytes: 1000000000},
			},
		},
	}
	p, ctx := newPlacementEngine(t, inv)
	affinity, err := p.NodeAffinity(ctx, "nvme")
	if err != nil {
		t.Fatalf("NodeAffinity() error = %v", err)
	}
	if affinity != nil {
		t.Errorf("expected nil affinity for pending devices, got %v", affinity)
	}
}

func TestPlacement_NodesWithCapacity_NoInventory(t *testing.T) {
	p, ctx := newPlacementEngine(t)
	nodes, err := p.NodesWithCapacity(ctx, "nvme", 100*1024*1024*1024)
	if err != nil {
		t.Fatalf("NodesWithCapacity() error = %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(nodes))
	}
}

func TestPlacement_NodesWithCapacity_MeetsThreshold(t *testing.T) {
	inv := &nestv1.HardwareInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "inv-capacity", Namespace: "default"},
		Spec: nestv1.HardwareInventorySpec{
			Node: "node-big",
			Devices: []nestv1.DeviceSpec{
				{Name: "/dev/nvme0n1", Class: "nvme", State: nestv1.DeviceStateActive, CapacityBytes: 2000000000000},
			},
		},
	}
	p, ctx := newPlacementEngine(t, inv)
	// 80% of 2TB = 1.6TB; ask for 1TB
	nodes, err := p.NodesWithCapacity(ctx, "nvme", 1000000000000)
	if err != nil {
		t.Fatalf("NodesWithCapacity() error = %v", err)
	}
	if len(nodes) != 1 || nodes[0] != "node-big" {
		t.Errorf("expected [node-big], got %v", nodes)
	}
}

func TestPlacement_NodesWithCapacity_BelowThreshold(t *testing.T) {
	inv := &nestv1.HardwareInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "inv-small", Namespace: "default"},
		Spec: nestv1.HardwareInventorySpec{
			Node: "node-small",
			Devices: []nestv1.DeviceSpec{
				{Name: "/dev/sda", Class: "hdd", State: nestv1.DeviceStateDark, CapacityBytes: 100000000},
			},
		},
	}
	p, ctx := newPlacementEngine(t, inv)
	// Ask for more than available
	nodes, err := p.NodesWithCapacity(ctx, "hdd", 1000000000000)
	if err != nil {
		t.Fatalf("NodesWithCapacity() error = %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes below threshold, got %v", nodes)
	}
}

func TestPlacement_PoolSummary_Empty(t *testing.T) {
	p, ctx := newPlacementEngine(t)
	summary, err := p.PoolSummary(ctx)
	if err != nil {
		t.Fatalf("PoolSummary() error = %v", err)
	}
	if len(summary) != 0 {
		t.Errorf("expected empty summary, got %v", summary)
	}
}

func TestPlacement_PoolSummary_WithDevices(t *testing.T) {
	inv := &nestv1.HardwareInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "inv-pool", Namespace: "default"},
		Spec: nestv1.HardwareInventorySpec{
			Node: "node-pool",
			Devices: []nestv1.DeviceSpec{
				{Name: "/dev/nvme0n1", Class: "nvme", State: nestv1.DeviceStateActive, CapacityBytes: 1000000000},
				{Name: "/dev/nvme1n1", Class: "nvme", State: nestv1.DeviceStateDark, CapacityBytes: 2000000000},
				{Name: "/dev/sda", Class: "hdd", State: nestv1.DeviceStateFailed, CapacityBytes: 4000000000},
				{Name: "/dev/sdb", Class: "", State: nestv1.DeviceStateActive, CapacityBytes: 500000000}, // no class, skipped
			},
		},
	}
	p, ctx := newPlacementEngine(t, inv)
	summary, err := p.PoolSummary(ctx)
	if err != nil {
		t.Fatalf("PoolSummary() error = %v", err)
	}
	if _, ok := summary["nvme"]; !ok {
		t.Error("expected 'nvme' in pool summary")
	}
	if _, ok := summary["hdd"]; !ok {
		t.Error("expected 'hdd' in pool summary")
	}
	if _, ok := summary[""]; ok {
		t.Error("empty class should not appear in summary")
	}
	nvme := summary["nvme"]
	if nvme.ActiveDevices != 1 {
		t.Errorf("nvme.ActiveDevices = %d, want 1", nvme.ActiveDevices)
	}
	if nvme.DarkDevices != 1 {
		t.Errorf("nvme.DarkDevices = %d, want 1", nvme.DarkDevices)
	}
	hdd := summary["hdd"]
	if hdd.FailedDevices != 1 {
		t.Errorf("hdd.FailedDevices = %d, want 1", hdd.FailedDevices)
	}
}

func TestPlacement_HardwareClassFromDataResourceClass(t *testing.T) {
	// nil DR
	if got := HardwareClassFromDataResourceClass(nil); got != "" {
		t.Errorf("nil DR = %q, want empty", got)
	}
	// DR without annotations
	dr := newDR("dr", "tenant", "postgres")
	if got := HardwareClassFromDataResourceClass(dr); got != "" {
		t.Errorf("no annotations = %q, want empty", got)
	}
	// DR with annotation
	dr.Spec.Annotations = map[string]string{
		"nest.penguintech.io/hardware-class": "nvme",
	}
	if got := HardwareClassFromDataResourceClass(dr); got != "nvme" {
		t.Errorf("with annotation = %q, want nvme", got)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// MariaDB / MySQL namespace helpers (standalone)
// ─────────────────────────────────────────────────────────────────────────────

func TestMariaDB_EnsureNamespace(t *testing.T) {
	r, ctx := reconcilerFor(t)
	if err := r.ensureMariaDBNamespace(ctx, "mdb-ns-test"); err != nil {
		t.Fatalf("ensureMariaDBNamespace error = %v", err)
	}
	// Idempotent
	if err := r.ensureMariaDBNamespace(ctx, "mdb-ns-test"); err != nil {
		t.Fatalf("ensureMariaDBNamespace (already exists) error = %v", err)
	}
}

func TestMySQL_EnsureNamespace(t *testing.T) {
	r, ctx := reconcilerFor(t)
	if err := r.ensureMySQLNamespace(ctx, "mysql-ns-test"); err != nil {
		t.Fatalf("ensureMySQLNamespace error = %v", err)
	}
	// Idempotent
	if err := r.ensureMySQLNamespace(ctx, "mysql-ns-test"); err != nil {
		t.Fatalf("ensureMySQLNamespace (already exists) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// MariaDB DBLB config
// ─────────────────────────────────────────────────────────────────────────────

func TestMariaDB_DblbConfig(t *testing.T) {
	dr := newDR("mdb-dblb", "tenant-mdbdblb", "mariadb")
	r, ctx := reconcilerFor(t, dr)
	if err := r.ensureMariaDBNamespace(ctx, dr.Spec.Tenant); err != nil {
		t.Fatalf("ensureMariaDBNamespace error = %v", err)
	}
	if err := r.reconcileDblbConfigMariaDB(ctx, dr); err != nil {
		t.Fatalf("reconcileDblbConfigMariaDB (create) error = %v", err)
	}
	if err := r.reconcileDblbConfigMariaDB(ctx, dr); err != nil {
		t.Fatalf("reconcileDblbConfigMariaDB (update) error = %v", err)
	}
}

func TestMySQL_DblbConfig(t *testing.T) {
	dr := newDR("mysql-dblb", "tenant-mysqldblb", "mysql")
	r, ctx := reconcilerFor(t, dr)
	if err := r.ensureMySQLNamespace(ctx, dr.Spec.Tenant); err != nil {
		t.Fatalf("ensureMySQLNamespace error = %v", err)
	}
	if err := r.reconcileDblbConfigMySQL(ctx, dr); err != nil {
		t.Fatalf("reconcileDblbConfigMySQL (create) error = %v", err)
	}
	if err := r.reconcileDblbConfigMySQL(ctx, dr); err != nil {
		t.Fatalf("reconcileDblbConfigMySQL (update) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// BoolPtr helper (used across reconcilers)
// ─────────────────────────────────────────────────────────────────────────────

func TestBoolPtr(t *testing.T) {
	b := boolPtr(true)
	if b == nil || !*b {
		t.Error("boolPtr(true) should return non-nil pointer to true")
	}
	b2 := boolPtr(false)
	if b2 == nil || *b2 {
		t.Error("boolPtr(false) should return non-nil pointer to false")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// iSCSI invalid storage size
// ─────────────────────────────────────────────────────────────────────────────

func TestISCSI_Reconcile_InvalidStorageSize(t *testing.T) {
	os.Setenv("ISCSI_GATEWAY_ENDPOINT", "http://localhost:1") //nolint:errcheck
	defer os.Unsetenv("ISCSI_GATEWAY_ENDPOINT")               //nolint:errcheck

	dr := newDR("iscsi-badsize", "tenant-iscsi5", "iscsi")
	dr.Spec.Size = &nestv1.ResourceSize{Storage: "not-valid-quantity"}
	r, ctx := reconcilerFor(t, dr)
	_, err := r.Reconcile(ctx, reqFor(dr))
	// Invalid storage size should produce an error
	if err == nil {
		t.Error("expected error for invalid storage size, got nil")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// "Already exists" tests — exercises the update/status-check branches
// These pre-create the unstructured external CRs in the fake client.
// ─────────────────────────────────────────────────────────────────────────────

// newUnstructuredCR creates a minimal unstructured CR for testing.
// All numeric values use int64 (JSON-safe for DeepCopyJSONValue).
func newUnstructuredCR(gvk schema.GroupVersionKind, name, namespace string, statusFields map[string]interface{}) *unstructured.Unstructured {
	u := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": gvk.Group + "/" + gvk.Version,
			"kind":       gvk.Kind,
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
	if statusFields != nil {
		u.Object["status"] = statusFields
	}
	return u
}

func reconcilerForWithUnstructured(t *testing.T, dr *nestv1.DataResource, crs ...*unstructured.Unstructured) (*DataResourceReconciler, context.Context) {
	t.Helper()
	scheme := newTestScheme(t)
	builder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&nestv1.DataResource{}).
		WithObjects(dr)
	for _, cr := range crs {
		builder = builder.WithObjects(cr)
	}
	fakeClient := builder.Build()
	r := &DataResourceReconciler{Client: fakeClient, Scheme: scheme}
	return r, context.Background()
}

func TestClickhouse_Reconcile_ExistingNotReady(t *testing.T) {
	dr := newDR("ch-existing", "tenant-che", "clickhouse")
	chiGVK := schema.GroupVersionKind{Group: "clickhouse.altinity.com", Version: "v1", Kind: "ClickHouseInstallation"}
	existing := newUnstructuredCR(chiGVK, clickhouseClusterName(dr), dr.Spec.Tenant, map[string]interface{}{
		"status": "InProgress",
	})
	r, ctx := reconcilerForWithUnstructured(t, dr, existing)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	var updated nestv1.DataResource
	if err := r.Client.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if updated.Status.Phase != nestv1.PhaseProvisioning {
		t.Errorf("Status.Phase = %v, want Provisioning", updated.Status.Phase)
	}
}

func TestClickhouse_Reconcile_ExistingReady(t *testing.T) {
	dr := newDR("ch-ready", "tenant-chready", "clickhouse")
	chiGVK := schema.GroupVersionKind{Group: "clickhouse.altinity.com", Version: "v1", Kind: "ClickHouseInstallation"}
	existing := newUnstructuredCR(chiGVK, clickhouseClusterName(dr), dr.Spec.Tenant, map[string]interface{}{
		"status": "Completed",
	})
	r, ctx := reconcilerForWithUnstructured(t, dr, existing)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	var updated nestv1.DataResource
	if err := r.Client.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if updated.Status.Phase != nestv1.PhaseReady {
		t.Errorf("Status.Phase = %v, want Ready", updated.Status.Phase)
	}
}

func TestKafka_Reconcile_ExistingNotReady(t *testing.T) {
	dr := newDR("kafka-existing", "tenant-kafkae", "kafka")
	kafkaGVK := schema.GroupVersionKind{Group: "kafka.strimzi.io", Version: "v1beta2", Kind: "Kafka"}
	existing := newUnstructuredCR(kafkaGVK, kafkaClusterName(dr), dr.Spec.Tenant, map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{"type": "Ready", "status": "False"},
		},
	})
	r, ctx := reconcilerForWithUnstructured(t, dr, existing)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	var updated nestv1.DataResource
	if err := r.Client.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if updated.Status.Phase != nestv1.PhaseProvisioning {
		t.Errorf("Status.Phase = %v, want Provisioning", updated.Status.Phase)
	}
}

func TestKafka_Reconcile_ExistingReady(t *testing.T) {
	dr := newDR("kafka-ready", "tenant-kafkaready", "kafka")
	kafkaGVK := schema.GroupVersionKind{Group: "kafka.strimzi.io", Version: "v1beta2", Kind: "Kafka"}
	existing := newUnstructuredCR(kafkaGVK, kafkaClusterName(dr), dr.Spec.Tenant, map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{"type": "Ready", "status": "True"},
		},
	})
	r, ctx := reconcilerForWithUnstructured(t, dr, existing)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	var updated nestv1.DataResource
	if err := r.Client.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if updated.Status.Phase != nestv1.PhaseReady {
		t.Errorf("Status.Phase = %v, want Ready", updated.Status.Phase)
	}
}

func TestOpenSearch_Reconcile_ExistingRunning(t *testing.T) {
	dr := newDR("os-running", "tenant-osrun", "search")
	osGVK := schema.GroupVersionKind{Group: "opensearch.opster.io", Version: "v1", Kind: "OpenSearchCluster"}
	existing := newUnstructuredCR(osGVK, opensearchClusterName(dr), dr.Spec.Tenant, map[string]interface{}{
		"phase": "RUNNING",
	})
	r, ctx := reconcilerForWithUnstructured(t, dr, existing)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	var updated nestv1.DataResource
	if err := r.Client.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if updated.Status.Phase != nestv1.PhaseReady {
		t.Errorf("Status.Phase = %v, want Ready", updated.Status.Phase)
	}
}

func TestOpenSearch_Reconcile_ExistingNotRunning(t *testing.T) {
	dr := newDR("os-pending", "tenant-ospend", "search")
	osGVK := schema.GroupVersionKind{Group: "opensearch.opster.io", Version: "v1", Kind: "OpenSearchCluster"}
	existing := newUnstructuredCR(osGVK, opensearchClusterName(dr), dr.Spec.Tenant, map[string]interface{}{
		"phase": "INITIALIZED",
	})
	r, ctx := reconcilerForWithUnstructured(t, dr, existing)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	var updated nestv1.DataResource
	if err := r.Client.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if updated.Status.Phase != nestv1.PhaseProvisioning {
		t.Errorf("Status.Phase = %v, want Provisioning", updated.Status.Phase)
	}
}

func TestVector_Reconcile_ExistingReady(t *testing.T) {
	dr := newDR("vec-ready", "tenant-vecready", "vector")
	pgGVK := schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "Cluster"}
	existing := newUnstructuredCR(pgGVK, vectorClusterName(dr), dr.Spec.Tenant, map[string]interface{}{
		"readyInstances": int64(2),
	})
	r, ctx := reconcilerForWithUnstructured(t, dr, existing)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestVector_Reconcile_ExistingNotReady(t *testing.T) {
	dr := newDR("vec-notready", "tenant-vecnotready", "vector")
	pgGVK := schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "Cluster"}
	existing := newUnstructuredCR(pgGVK, vectorClusterName(dr), dr.Spec.Tenant, map[string]interface{}{
		"readyInstances": int64(0),
	})
	r, ctx := reconcilerForWithUnstructured(t, dr, existing)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	var updated nestv1.DataResource
	if err := r.Client.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if updated.Status.Phase != nestv1.PhaseProvisioning {
		t.Errorf("Status.Phase = %v, want Provisioning", updated.Status.Phase)
	}
}

func TestMariaDB_Reconcile_ExistingProvisioning(t *testing.T) {
	// When MariaDB cluster exists but is not ready
	dr := newDR("mdb-existing", "tenant-mdbe", "mariadb")
	mdbGVK := schema.GroupVersionKind{Group: "mariadb.mmontes.io", Version: "v1alpha1", Kind: "MariaDB"}
	existing := newUnstructuredCR(mdbGVK, mariadbClusterName(dr), dr.Spec.Tenant, map[string]interface{}{
		"ready": false,
	})
	r, ctx := reconcilerForWithUnstructured(t, dr, existing)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestMariaDB_Reconcile_ExistingReady(t *testing.T) {
	dr := newDR("mdb-rdy", "tenant-mdbr", "mariadb")
	mdbGVK := schema.GroupVersionKind{Group: "mariadb.mmontes.io", Version: "v1alpha1", Kind: "MariaDB"}
	existing := newUnstructuredCR(mdbGVK, mariadbClusterName(dr), dr.Spec.Tenant, map[string]interface{}{
		"ready": true,
	})
	r, ctx := reconcilerForWithUnstructured(t, dr, existing)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestMySQL_Reconcile_ExistingProvisioning(t *testing.T) {
	dr := newDR("mysql-existing", "tenant-mysqle", "mysql")
	mysqlGVK := schema.GroupVersionKind{Group: "mysql.oracle.com", Version: "v2", Kind: "InnoDBCluster"}
	existing := newUnstructuredCR(mysqlGVK, mysqlClusterName(dr), dr.Spec.Tenant, map[string]interface{}{
		"status": "PENDING",
	})
	r, ctx := reconcilerForWithUnstructured(t, dr, existing)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestMySQL_Reconcile_ExistingReady(t *testing.T) {
	dr := newDR("mysql-rdy", "tenant-mysqlr", "mysql")
	mysqlGVK := schema.GroupVersionKind{Group: "mysql.oracle.com", Version: "v2", Kind: "InnoDBCluster"}
	existing := newUnstructuredCR(mysqlGVK, mysqlClusterName(dr), dr.Spec.Tenant, map[string]interface{}{
		"status": "ONLINE",
	})
	r, ctx := reconcilerForWithUnstructured(t, dr, existing)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestFerretDB_Reconcile_PGExistsDeploymentNew(t *testing.T) {
	dr := newDR("fdb-pgexists", "tenant-fdbe", "rockfs")
	pgGVK := schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "Cluster"}
	pgCluster := newUnstructuredCR(pgGVK, ferretdbPGClusterName(dr), dr.Spec.Tenant, map[string]interface{}{
		"readyInstances": int64(1),
	})
	r, ctx := reconcilerForWithUnstructured(t, dr, pgCluster)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestFerretDB_Reconcile_PGExistsDeploymentExists(t *testing.T) {
	dr := newDR("fdb-all", "tenant-fdbfull", "rockfs")
	pgGVK := schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "Cluster"}
	pgCluster := newUnstructuredCR(pgGVK, ferretdbPGClusterName(dr), dr.Spec.Tenant, map[string]interface{}{
		"readyInstances": int64(1),
	})
	r, ctx := reconcilerForWithUnstructured(t, dr, pgCluster)
	// First call creates deployment
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}
	// Second call: deployment exists
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
}

func TestIceberg_Reconcile_PGExistsDeploymentNew(t *testing.T) {
	dr := newDR("ice-pgexists", "tenant-icee", "lakehouse/iceberg")
	pgGVK := schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "Cluster"}
	pgCluster := newUnstructuredCR(pgGVK, icebergPGClusterName(dr), icebergNamespace(dr), map[string]interface{}{
		"readyInstances": int64(1),
	})
	r, ctx := reconcilerForWithUnstructured(t, dr, pgCluster)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestIceberg_Reconcile_AllExistsServiceNew(t *testing.T) {
	dr := newDR("ice-allexists", "tenant-icefull", "lakehouse/iceberg")
	pgGVK := schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "Cluster"}
	pgCluster := newUnstructuredCR(pgGVK, icebergPGClusterName(dr), icebergNamespace(dr), map[string]interface{}{
		"readyInstances": int64(1),
	})
	r, ctx := reconcilerForWithUnstructured(t, dr, pgCluster)
	// First call creates Deployment (returns Provisioning)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}
	// Second call: Deployment exists, creates Service
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
	// Third call: everything exists, check status
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("third Reconcile() error = %v", err)
	}
}

func TestTrino_Reconcile_AllExistsIdempotent(t *testing.T) {
	dr := newDR("trino-idem2", "tenant-trinoidem", "warehouse/trino")
	dr.Spec.Annotations = map[string]string{}
	r, ctx := reconcilerFor(t, dr)
	// Create configmaps, deployments, and service on first call
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}
	// Subsequent calls should be idempotent
	for i := 0; i < 3; i++ {
		if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
			t.Fatalf("Reconcile() call %d error = %v", i+2, err)
		}
	}
}

func TestTimeseries_Reconcile_ExistingStatefulSet(t *testing.T) {
	dr := newDR("ts-existing", "tenant-tse", "timeseries")
	r, ctx := reconcilerFor(t, dr)
	// First call creates StatefulSet and Service
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}
	// Second call: resources exist, check status
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
	var updated nestv1.DataResource
	if err := r.Client.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	// After second reconcile StatefulSet exists but not ready (no pods) → Provisioning
	if updated.Status.Phase != nestv1.PhaseProvisioning {
		t.Errorf("Status.Phase = %v, want Provisioning", updated.Status.Phase)
	}
}

func TestFilesystem_Reconcile_ExistingPVC(t *testing.T) {
	dr := newDR("fs-existing", "tenant-fse", "filesystem")
	r, ctx := reconcilerFor(t, dr)
	// First reconcile creates PVC
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}
	// Second reconcile: PVC exists
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
}

func TestNFS_Delete_NotFoundAnnotation(t *testing.T) {
	// Test delete when annotation key is missing (empty annotations map)
	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "nfs-del-emptymap",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
			Annotations:       map[string]string{"other-key": "value"},
		},
		Spec: nestv1.DataResourceSpec{Type: "nfs", Tenant: "tenant-nfs-del3"},
	}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete/no-export-id-annotation) error = %v", err)
	}
}

func TestISCSI_Delete_NotFoundAnnotation(t *testing.T) {
	// Test delete when iscsi-target-id annotation key is missing
	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "iscsi-del-nokey",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
			Annotations:       map[string]string{"other": "val"},
		},
		Spec: nestv1.DataResourceSpec{Type: "iscsi", Tenant: "tenant-iscsi-del3"},
	}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete/no-target-id-annotation) error = %v", err)
	}
}

func TestNFS_Delete_GatewayNotFound(t *testing.T) {
	// Test delete when gateway returns 404 (idempotent)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	t.Setenv("NFS_GATEWAY_ENDPOINT", srv.URL)

	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "nfs-del-404",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
			Annotations: map[string]string{
				"nest.penguintech.io/nfs-export-id": "export-404",
			},
		},
		Spec: nestv1.DataResourceSpec{Type: "nfs", Tenant: "tenant-nfs-del4"},
	}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete/gateway-404) error = %v", err)
	}
}

func TestISCSI_Delete_GatewayNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	t.Setenv("ISCSI_GATEWAY_ENDPOINT", srv.URL)

	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "iscsi-del-404",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
			Annotations: map[string]string{
				"nest.penguintech.io/iscsi-target-id": "target-404",
			},
		},
		Spec: nestv1.DataResourceSpec{Type: "iscsi", Tenant: "tenant-iscsi-del4"},
	}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete/gateway-404) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Keyvalue StorageSize additional coverage
// ─────────────────────────────────────────────────────────────────────────────

func TestKeyvalue_StorageSize_WithValidSize(t *testing.T) {
	dr := newDR("kv-size", "tenant-kv", "keyvalue")
	dr.Spec.Size = &nestv1.ResourceSize{Storage: "32Gi"}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestKeyvalue_StorageSize_WithHa(t *testing.T) {
	dr := newDR("kv-ha", "tenant-kvha", "keyvalue")
	dr.Spec.HA = true
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(HA) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Keyvalue — idempotent (ConfigMap already exists → update branch)
// ─────────────────────────────────────────────────────────────────────────────

func TestKeyvalue_IdempotentWithExistingConfigMap(t *testing.T) {
	dr := newDR("kv-idem", "tenant-kvid", "keyvalue")

	// Pre-create the ConfigMap so reconciler takes the update branch
	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      keyvalueConfigMapName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Data: map[string]string{"valkey.conf": "old"},
	}
	r, ctx := reconcilerFor(t, dr, existingCM)
	// First reconcile: creates Service + StatefulSet (ConfigMap gets updated)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestKeyvalue_ReadyState(t *testing.T) {
	dr := newDR("kv-ready", "tenant-kvready", "keyvalue")

	// Pre-create ConfigMap, Service, and StatefulSet (ReadyReplicas=1)
	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      keyvalueConfigMapName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Data: map[string]string{"valkey.conf": "bind 0.0.0.0"},
	}
	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      keyvalueServiceName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	existingSS := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      keyvalueStatefulSetName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Status: appsv1.StatefulSetStatus{ReadyReplicas: 1},
	}
	r, ctx := reconcilerFor(t, dr, existingCM, existingSvc, existingSS)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(ready) error = %v", err)
	}
}

func TestKeyvalue_StatefulSetNotReady(t *testing.T) {
	dr := newDR("kv-notready", "tenant-kvnotready", "keyvalue")

	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      keyvalueConfigMapName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Data: map[string]string{"valkey.conf": "bind 0.0.0.0"},
	}
	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      keyvalueServiceName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	replicas := int32(1)
	existingSS := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      keyvalueStatefulSetName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Spec:   appsv1.StatefulSetSpec{Replicas: &replicas},
		Status: appsv1.StatefulSetStatus{ReadyReplicas: 0}, // NOT ready
	}
	r, ctx := reconcilerFor(t, dr, existingCM, existingSvc, existingSS)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(not ready) error = %v", err)
	}
}

func TestKeyvalue_WithCustomReplicas(t *testing.T) {
	dr := newDR("kv-replicas", "tenant-kvreplicas", "keyvalue")
	dr.Spec.Replicas = &nestv1.ReplicaConfig{
		Write: &nestv1.ReplicaCountSpec{Default: 3},
	}
	r, ctx := reconcilerFor(t, dr)
	// replicas > 1 is intentionally rejected (no silent split-brain Valkey); Reconcile should surface the error.
	if _, err := r.Reconcile(ctx, reqFor(dr)); err == nil {
		t.Fatalf("Reconcile(replicas>1) should reject unsupported replication, got nil error")
	}
}

func TestKeyvalue_Delete_WithExistingResources(t *testing.T) {
	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "kv-del-exist",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{Type: "keyvalue", Tenant: "tenant-kvdelexist"},
	}

	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      keyvalueConfigMapName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      keyvalueServiceName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	existingSS := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      keyvalueStatefulSetName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	r, ctx := reconcilerFor(t, dr, existingCM, existingSvc, existingSS)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete existing) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Timeseries — idempotent (Service exists, StatefulSet ready)
// ─────────────────────────────────────────────────────────────────────────────

func TestTimeseries_IdempotentWithExistingService(t *testing.T) {
	dr := newDR("ts-idem", "tenant-tsidem", "timeseries")

	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      timeseriesServiceName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	r, ctx := reconcilerFor(t, dr, existingSvc)
	// Service exists — reconciler should create StatefulSet
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestTimeseries_ReadyState(t *testing.T) {
	dr := newDR("ts-ready", "tenant-tsready", "timeseries")

	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      timeseriesServiceName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	replicas := int32(1)
	existingSS := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      timeseriesStatefulSetName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Spec:   appsv1.StatefulSetSpec{Replicas: &replicas},
		Status: appsv1.StatefulSetStatus{ReadyReplicas: 1},
	}
	r, ctx := reconcilerFor(t, dr, existingSvc, existingSS)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(ready) error = %v", err)
	}
}

func TestTimeseries_Delete_WithExistingResources(t *testing.T) {
	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "ts-del-exist",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{Type: "timeseries", Tenant: "tenant-tsdelexist"},
	}

	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      timeseriesServiceName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	existingSS := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      timeseriesStatefulSetName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	r, ctx := reconcilerFor(t, dr, existingSvc, existingSS)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete existing) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Filesystem — PVC bound (Ready state) + custom storage class
// ─────────────────────────────────────────────────────────────────────────────

func TestFilesystem_PVCBound_ReadyState(t *testing.T) {
	dr := newDR("fs-bound", "tenant-fsbound", "filesystem")

	existingPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      filesystemPVCName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimBound,
		},
	}
	r, ctx := reconcilerFor(t, dr, existingPVC)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(PVC bound) error = %v", err)
	}
}

func TestFilesystem_PVCPending_Provisioning(t *testing.T) {
	dr := newDR("fs-pend", "tenant-fspend", "filesystem")

	existingPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      filesystemPVCName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimPending,
		},
	}
	r, ctx := reconcilerFor(t, dr, existingPVC)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(PVC pending) error = %v", err)
	}
}

func TestFilesystem_CustomStorageClass(t *testing.T) {
	dr := newDR("fs-sc", "tenant-fssc", "filesystem")
	dr.Annotations = map[string]string{
		"nest.penguintech.io/storage-class": "local-path",
	}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(custom SC) error = %v", err)
	}
}

func TestFilesystem_Delete_WithExistingPVC(t *testing.T) {
	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "fs-del-exist",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{Type: "filesystem", Tenant: "tenant-fsdelexist"},
	}

	existingPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      filesystemPVCName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	r, ctx := reconcilerFor(t, dr, existingPVC)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete PVC) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Trino — partial + full idempotency paths
// ─────────────────────────────────────────────────────────────────────────────

func TestTrino_CoordinatorCMExists_UpdateBranch(t *testing.T) {
	dr := newDR("trino-cm-upd", "tenant-trinocm", "warehouse/trino")

	// Pre-create coordinator CM to trigger update branch
	existingCoordCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoCoordinatorConfigMapName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Data: map[string]string{"config.properties": "old-data"},
	}
	r, ctx := reconcilerFor(t, dr, existingCoordCM)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(coordinator CM update) error = %v", err)
	}
}

func TestTrino_BothCMsExist_UpdateBranches(t *testing.T) {
	dr := newDR("trino-cms-upd", "tenant-trinocms", "warehouse/trino")

	existingCoordCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoCoordinatorConfigMapName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Data: map[string]string{"config.properties": "old-coordinator"},
	}
	existingWorkerCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoWorkerConfigMapName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Data: map[string]string{"config.properties": "old-worker"},
	}
	r, ctx := reconcilerFor(t, dr, existingCoordCM, existingWorkerCM)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(both CMs update) error = %v", err)
	}
}

func TestTrino_AllResourcesExist_ReadyCoordinator(t *testing.T) {
	dr := newDR("trino-ready", "tenant-trinoready", "warehouse/trino")
	replicas := int32(2)

	existingCoordCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoCoordinatorConfigMapName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Data: map[string]string{"config.properties": "coordinator=true"},
	}
	existingWorkerCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoWorkerConfigMapName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Data: map[string]string{"config.properties": "coordinator=false"},
	}
	existingCoordDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoCoordinatorName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Status: appsv1.DeploymentStatus{AvailableReplicas: 1},
	}
	existingWorkerDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoWorkerName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Spec:   appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{AvailableReplicas: 2},
	}
	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoServiceName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	r, ctx := reconcilerFor(t, dr, existingCoordCM, existingWorkerCM, existingCoordDeploy, existingWorkerDeploy, existingSvc)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(all resources ready) error = %v", err)
	}
}

func TestTrino_CoordinatorReady_WorkersNotReady(t *testing.T) {
	dr := newDR("trino-coord-rdy", "tenant-trinocrdy", "warehouse/trino")
	replicas := int32(2)

	existingCoordCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoCoordinatorConfigMapName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Data: map[string]string{"config.properties": "coordinator=true"},
	}
	existingWorkerCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoWorkerConfigMapName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Data: map[string]string{"config.properties": "coordinator=false"},
	}
	existingCoordDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoCoordinatorName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Status: appsv1.DeploymentStatus{AvailableReplicas: 1},
	}
	existingWorkerDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoWorkerName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Spec:   appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{AvailableReplicas: 0}, // workers not ready
	}
	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoServiceName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	r, ctx := reconcilerFor(t, dr, existingCoordCM, existingWorkerCM, existingCoordDeploy, existingWorkerDeploy, existingSvc)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(coordinator ready, workers not) error = %v", err)
	}
}

func TestTrino_CoordinatorNotReady(t *testing.T) {
	dr := newDR("trino-coord-notready", "tenant-trinocoord0", "warehouse/trino")
	replicas := int32(2)

	existingCoordCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoCoordinatorConfigMapName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Data: map[string]string{"config.properties": "coordinator=true"},
	}
	existingWorkerCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoWorkerConfigMapName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Data: map[string]string{"config.properties": "coordinator=false"},
	}
	existingCoordDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoCoordinatorName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Status: appsv1.DeploymentStatus{AvailableReplicas: 0}, // coordinator NOT ready
	}
	existingWorkerDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoWorkerName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Spec:   appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{AvailableReplicas: 2},
	}
	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoServiceName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	r, ctx := reconcilerFor(t, dr, existingCoordCM, existingWorkerCM, existingCoordDeploy, existingWorkerDeploy, existingSvc)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(coordinator not ready) error = %v", err)
	}
}

func TestTrino_Delete_WithExistingResources(t *testing.T) {
	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "trino-del-exist",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{Type: "warehouse/trino", Tenant: "tenant-trinodelexist"},
	}

	coordDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoCoordinatorName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	workerDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoWorkerName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoServiceName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	coordCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoCoordinatorConfigMapName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	workerCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoWorkerConfigMapName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	r, ctx := reconcilerFor(t, dr, coordDeploy, workerDeploy, svc, coordCM, workerCM)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete with existing) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Trino helper function truncation coverage
// ─────────────────────────────────────────────────────────────────────────────

func TestTrino_LongNameTruncated_HelperFunctions(t *testing.T) {
	// Name that produces >63 chars after formatting
	longName := "this-is-a-very-long-dataresource-name-to-exceed"
	drLong := newDR(longName, "tenant-trino-long", "warehouse/trino")
	drShort := newDR("short", "t", "warehouse/trino")

	// Test with long name (triggers truncation branch)
	coordName := trinoCoordinatorName(drLong)
	if len(coordName) > 63 {
		t.Errorf("trinoCoordinatorName too long: %d chars", len(coordName))
	}
	workerName := trinoWorkerName(drLong)
	if len(workerName) > 63 {
		t.Errorf("trinoWorkerName too long: %d chars", len(workerName))
	}
	svcName := trinoServiceName(drLong)
	if len(svcName) > 63 {
		t.Errorf("trinoServiceName too long: %d chars", len(svcName))
	}
	coordCMName := trinoCoordinatorConfigMapName(drLong)
	if len(coordCMName) > 63 {
		t.Errorf("trinoCoordinatorConfigMapName too long: %d chars", len(coordCMName))
	}
	workerCMName := trinoWorkerConfigMapName(drLong)
	if len(workerCMName) > 63 {
		t.Errorf("trinoWorkerConfigMapName too long: %d chars", len(workerCMName))
	}

	// Test with short name (triggers non-truncation branch → return name)
	_ = trinoCoordinatorName(drShort)
	_ = trinoWorkerName(drShort)
	_ = trinoServiceName(drShort)
	_ = trinoCoordinatorConfigMapName(drShort)
	_ = trinoWorkerConfigMapName(drShort)

	// Also reconcile to test the full path with long name
	r, ctx := reconcilerFor(t, drLong)
	if _, err := r.Reconcile(ctx, reqFor(drLong)); err != nil {
		t.Fatalf("Reconcile(long name) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Iceberg — deployment exists, service creation, ready state
// ─────────────────────────────────────────────────────────────────────────────

func TestIceberg_DeploymentExists_ServiceCreated(t *testing.T) {
	dr := newDR("ice-dep-exist", "tenant-icedep", "lakehouse/iceberg")

	// Pre-create PG cluster as ready + deployment
	pgGVK := schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "Cluster"}
	pgCluster := newUnstructuredCR(pgGVK, icebergPGClusterName(dr), dr.Spec.Tenant, map[string]interface{}{
		"readyInstances": int64(1),
	})

	existingDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      icebergRestName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Status: appsv1.DeploymentStatus{AvailableReplicas: 1},
	}
	r, ctx := reconcilerForWithUnstructured(t, dr, pgCluster)
	// Add the deployment to the reconciler's client by creating it in the scheme
	if err := r.Create(ctx, existingDeploy); err != nil {
		t.Fatalf("pre-create deployment: %v", err)
	}
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(deployment exists) error = %v", err)
	}
}

func TestIceberg_DeploymentAndServiceExist_ReadyState(t *testing.T) {
	dr := newDR("ice-all-exist", "tenant-iceall", "lakehouse/iceberg")

	pgGVK := schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "Cluster"}
	pgCluster := newUnstructuredCR(pgGVK, icebergPGClusterName(dr), dr.Spec.Tenant, map[string]interface{}{
		"readyInstances": int64(1),
	})

	existingDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      icebergRestName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Status: appsv1.DeploymentStatus{AvailableReplicas: 1},
	}
	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      icebergRestName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	r, ctx := reconcilerForWithUnstructured(t, dr, pgCluster)
	if err := r.Create(ctx, existingDeploy); err != nil {
		t.Fatalf("pre-create deployment: %v", err)
	}
	if err := r.Create(ctx, existingSvc); err != nil {
		t.Fatalf("pre-create service: %v", err)
	}
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(all exist, ready) error = %v", err)
	}
}

func TestIceberg_Delete_WithExistingResources(t *testing.T) {
	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "ice-del-exist",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{Type: "lakehouse/iceberg", Tenant: "tenant-icedelexist"},
	}

	pgGVK := schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "Cluster"}
	pgCluster := newUnstructuredCR(pgGVK, icebergPGClusterName(dr), dr.Spec.Tenant, nil)

	existingDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      icebergRestName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      icebergRestName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	r, ctx := reconcilerForWithUnstructured(t, dr, pgCluster)
	if err := r.Create(ctx, existingDeploy); err != nil {
		t.Fatalf("pre-create deployment: %v", err)
	}
	if err := r.Create(ctx, existingSvc); err != nil {
		t.Fatalf("pre-create service: %v", err)
	}
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete with existing) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// NFS — additional error paths
// ─────────────────────────────────────────────────────────────────────────────

func TestNFS_GatewayInvalidJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`not-valid-json`)) // invalid JSON body
	}))
	defer srv.Close()
	t.Setenv("NFS_GATEWAY_ENDPOINT", srv.URL)

	dr := newDR("nfs-badjson", "tenant-nfsbadjson", "nfs")
	r, ctx := reconcilerFor(t, dr)
	_, err := r.Reconcile(ctx, reqFor(dr))
	if err == nil {
		t.Fatal("expected error for invalid JSON NFS gateway response")
	}
}

func TestNFS_GatewayConnectionRefused(t *testing.T) {
	// Use a closed server to trigger http.Post connection error
	t.Setenv("NFS_GATEWAY_ENDPOINT", "http://127.0.0.1:1") // port 1 is always refused

	dr := newDR("nfs-connrefused", "tenant-nfsconn", "nfs")
	r, ctx := reconcilerFor(t, dr)
	_, err := r.Reconcile(ctx, reqFor(dr))
	if err == nil {
		t.Fatal("expected error for NFS gateway connection refused")
	}
}

func TestNFS_GatewayReturnsNonCreated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	t.Setenv("NFS_GATEWAY_ENDPOINT", srv.URL)

	dr := newDR("nfs-500", "tenant-nfs500", "nfs")
	r, ctx := reconcilerFor(t, dr)
	_, err := r.Reconcile(ctx, reqFor(dr))
	if err == nil {
		t.Fatal("expected error for non-201 NFS gateway response")
	}
}

func TestNFS_GatewayMissingIDField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"path":"/some/path"}`)) // missing "id"
	}))
	defer srv.Close()
	t.Setenv("NFS_GATEWAY_ENDPOINT", srv.URL)

	dr := newDR("nfs-noid", "tenant-nfsnoid", "nfs")
	r, ctx := reconcilerFor(t, dr)
	_, err := r.Reconcile(ctx, reqFor(dr))
	if err == nil {
		t.Fatal("expected error for missing 'id' in NFS gateway response")
	}
}

func TestNFS_Delete_WithAnnotation_Non200Response(t *testing.T) {
	// The controller ignores delete errors (uses _ = r.reconcileNFSDelete), so no error returned from Reconcile.
	// But reconcileNFSDelete itself returns an error for non-200/404 — test it directly.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	t.Setenv("NFS_GATEWAY_ENDPOINT", srv.URL)

	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "nfs-del-500",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
			Annotations: map[string]string{
				"nest.penguintech.io/nfs-export-id": "export-500",
			},
		},
		Spec: nestv1.DataResourceSpec{Type: "nfs", Tenant: "tenant-nfsdel500"},
	}
	r, ctx := reconcilerFor(t, dr)
	// Delete errors now propagate (delete-safety): the finalizer must NOT be removed
	// when gateway cleanup fails, so Reconcile should surface the error.
	if _, err := r.Reconcile(ctx, reqFor(dr)); err == nil {
		t.Fatalf("Reconcile(delete/500) should propagate gateway error, got nil")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// iSCSI — additional error paths
// ─────────────────────────────────────────────────────────────────────────────

func TestISCSI_GatewayInvalidJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`not-valid-json`))
	}))
	defer srv.Close()
	t.Setenv("ISCSI_GATEWAY_ENDPOINT", srv.URL)

	dr := newDR("iscsi-badjson", "tenant-iscsibadjson", "iscsi")
	r, ctx := reconcilerFor(t, dr)
	_, err := r.Reconcile(ctx, reqFor(dr))
	if err == nil {
		t.Fatal("expected error for invalid JSON iSCSI gateway response")
	}
}

func TestISCSI_GatewayConnectionRefused(t *testing.T) {
	t.Setenv("ISCSI_GATEWAY_ENDPOINT", "http://127.0.0.1:1")

	dr := newDR("iscsi-connrefused", "tenant-iscsiconn", "iscsi")
	r, ctx := reconcilerFor(t, dr)
	_, err := r.Reconcile(ctx, reqFor(dr))
	if err == nil {
		t.Fatal("expected error for iSCSI gateway connection refused")
	}
}

func TestISCSI_GatewayReturnsNonCreated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	t.Setenv("ISCSI_GATEWAY_ENDPOINT", srv.URL)

	dr := newDR("iscsi-500", "tenant-iscsi500", "iscsi")
	r, ctx := reconcilerFor(t, dr)
	_, err := r.Reconcile(ctx, reqFor(dr))
	if err == nil {
		t.Fatal("expected error for non-201 iSCSI gateway response")
	}
}

func TestISCSI_GatewayMissingIDField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"iqn":"iqn.2024-01.io.penguintech:test"}`)) // missing "id"
	}))
	defer srv.Close()
	t.Setenv("ISCSI_GATEWAY_ENDPOINT", srv.URL)

	dr := newDR("iscsi-noid", "tenant-iscsino", "iscsi")
	r, ctx := reconcilerFor(t, dr)
	_, err := r.Reconcile(ctx, reqFor(dr))
	if err == nil {
		t.Fatal("expected error for missing 'id' in iSCSI gateway response")
	}
}

func TestISCSI_GatewayMissingIQNField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"target-001"}`)) // missing "iqn"
	}))
	defer srv.Close()
	t.Setenv("ISCSI_GATEWAY_ENDPOINT", srv.URL)

	dr := newDR("iscsi-noiqn", "tenant-iscsinoiqn", "iscsi")
	r, ctx := reconcilerFor(t, dr)
	_, err := r.Reconcile(ctx, reqFor(dr))
	if err == nil {
		t.Fatal("expected error for missing 'iqn' in iSCSI gateway response")
	}
}

func TestISCSI_Delete_Non200Response(t *testing.T) {
	// The controller ignores delete errors (uses _ = r.reconcileISCSIDelete), so no error returned from Reconcile.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	t.Setenv("ISCSI_GATEWAY_ENDPOINT", srv.URL)

	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "iscsi-del-500",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
			Annotations: map[string]string{
				"nest.penguintech.io/iscsi-target-id": "target-500",
			},
		},
		Spec: nestv1.DataResourceSpec{Type: "iscsi", Tenant: "tenant-iscsidel500"},
	}
	r, ctx := reconcilerFor(t, dr)
	// Delete errors now propagate (delete-safety): the finalizer must NOT be removed
	// when gateway cleanup fails, so Reconcile should surface the error.
	if _, err := r.Reconcile(ctx, reqFor(dr)); err == nil {
		t.Fatalf("Reconcile(delete/500) should propagate gateway error, got nil")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// FerretDB — deployment ready state
// ─────────────────────────────────────────────────────────────────────────────

func TestFerretDB_DeploymentExists_ReadyState(t *testing.T) {
	dr := newDR("fdb-ready", "tenant-fdbready", "rockfs")

	pgGVK := schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "Cluster"}
	pgCluster := newUnstructuredCR(pgGVK, ferretdbPGClusterName(dr), dr.Spec.Tenant, map[string]interface{}{
		"readyInstances": int64(1),
	})

	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ferretdbDeploymentName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	existingDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ferretdbDeploymentName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Status: appsv1.DeploymentStatus{AvailableReplicas: 1},
	}
	r, ctx := reconcilerForWithUnstructured(t, dr, pgCluster)
	if err := r.Create(ctx, existingSvc); err != nil {
		t.Fatalf("pre-create svc: %v", err)
	}
	if err := r.Create(ctx, existingDeploy); err != nil {
		t.Fatalf("pre-create deploy: %v", err)
	}
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(deployment ready) error = %v", err)
	}
}

func TestFerretDB_DeploymentExists_NotReady(t *testing.T) {
	dr := newDR("fdb-notready", "tenant-fdbnotready", "rockfs")

	pgGVK := schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "Cluster"}
	pgCluster := newUnstructuredCR(pgGVK, ferretdbPGClusterName(dr), dr.Spec.Tenant, map[string]interface{}{
		"readyInstances": int64(1),
	})

	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ferretdbDeploymentName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	existingDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ferretdbDeploymentName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Status: appsv1.DeploymentStatus{AvailableReplicas: 0},
	}
	r, ctx := reconcilerForWithUnstructured(t, dr, pgCluster)
	if err := r.Create(ctx, existingSvc); err != nil {
		t.Fatalf("pre-create svc: %v", err)
	}
	if err := r.Create(ctx, existingDeploy); err != nil {
		t.Fatalf("pre-create deploy: %v", err)
	}
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(deployment not ready) error = %v", err)
	}
}

func TestFerretDB_Delete_WithAllResources(t *testing.T) {
	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "fdb-del-all",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{Type: "rockfs", Tenant: "tenant-fdbdelall"},
	}

	pgGVK := schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "Cluster"}
	pgCluster := newUnstructuredCR(pgGVK, ferretdbPGClusterName(dr), dr.Spec.Tenant, nil)

	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ferretdbDeploymentName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	existingDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ferretdbDeploymentName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	r, ctx := reconcilerForWithUnstructured(t, dr, pgCluster)
	if err := r.Create(ctx, existingSvc); err != nil {
		t.Fatalf("pre-create svc: %v", err)
	}
	if err := r.Create(ctx, existingDeploy); err != nil {
		t.Fatalf("pre-create deploy: %v", err)
	}
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete all) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// MariaDB — existing cluster + DBLB ready state
// ─────────────────────────────────────────────────────────────────────────────

func TestMariaDB_ClusterReadyWithDblb(t *testing.T) {
	dr := newDR("mdb-rdy", "tenant-mdbrdy", "mariadb")

	mariadbGVK := schema.GroupVersionKind{Group: "k8s.mariadb.com", Version: "v1alpha1", Kind: "MariaDB"}
	existingCluster := newUnstructuredCR(mariadbGVK, mariadbClusterName(dr), dr.Spec.Tenant, map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{
				"type":   "Ready",
				"status": "True",
			},
		},
	})

	r, ctx := reconcilerForWithUnstructured(t, dr, existingCluster)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(mariadb ready with dblb) error = %v", err)
	}
}

func TestMariaDB_Delete_WithExistingCluster(t *testing.T) {
	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "mdb-del-exist",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{Type: "mariadb", Tenant: "tenant-mdbdelexist"},
	}

	mariadbGVK := schema.GroupVersionKind{Group: "k8s.mariadb.com", Version: "v1alpha1", Kind: "MariaDB"}
	existingCluster := newUnstructuredCR(mariadbGVK, mariadbClusterName(dr), dr.Spec.Tenant, nil)

	r, ctx := reconcilerForWithUnstructured(t, dr, existingCluster)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete mariadb) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// MySQL — existing cluster + ready state
// ─────────────────────────────────────────────────────────────────────────────

func TestMySQL_ClusterReadyWithDblb(t *testing.T) {
	dr := newDR("mysql-rdy", "tenant-mysqlrdy", "mysql")

	mysqlGVK := schema.GroupVersionKind{Group: "mysql.oracle.com", Version: "v2", Kind: "InnoDBCluster"}
	existingCluster := newUnstructuredCR(mysqlGVK, mysqlClusterName(dr), dr.Spec.Tenant, nil)
	// Set nested status field for ONLINE check
	_ = unstructured.SetNestedField(existingCluster.Object, "ONLINE", "status", "cluster", "status")

	r, ctx := reconcilerForWithUnstructured(t, dr, existingCluster)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(mysql ready) error = %v", err)
	}
}

func TestMySQL_Delete_WithExistingCluster(t *testing.T) {
	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "mysql-del-exist",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{Type: "mysql", Tenant: "tenant-mysqldelexist"},
	}

	mysqlGVK := schema.GroupVersionKind{Group: "mysql.oracle.com", Version: "v2", Kind: "InnoDBCluster"}
	existingCluster := newUnstructuredCR(mysqlGVK, mysqlClusterName(dr), dr.Spec.Tenant, nil)

	r, ctx := reconcilerForWithUnstructured(t, dr, existingCluster)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete mysql) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Iceberg helper function truncation
// ─────────────────────────────────────────────────────────────────────────────

func TestIceberg_HelperFunctionTruncation(t *testing.T) {
	longName := "this-is-a-very-long-dataresource-name-exceeds-sixty-chars"
	drLong := newDR(longName, "tenant-ice-long", "lakehouse/iceberg")
	drShort := newDR("short", "t", "lakehouse/iceberg")

	// Long name → truncation branch
	name := icebergRestName(drLong)
	if len(name) > 63 {
		t.Errorf("icebergRestName too long: %d chars", len(name))
	}

	// Short name → non-truncation branch
	_ = icebergRestName(drShort)
}

// ─────────────────────────────────────────────────────────────────────────────
// OpenSearch — long name truncation test
// ─────────────────────────────────────────────────────────────────────────────

func TestOpenSearch_LongNameTruncated(t *testing.T) {
	// Name > 63 chars after formatting triggers the truncation branch
	longName := "this-is-a-very-long-dataresource-name-exceeding-sixty-three-characters"
	drLong := newDR(longName, "tenant-os", "search")
	drShort := newDR("short", "t", "search")

	// Long name → truncation
	name := opensearchClusterName(drLong)
	if len(name) > 63 {
		t.Errorf("opensearchClusterName too long: %d chars", len(name))
	}
	// Short name → no truncation
	_ = opensearchClusterName(drShort)

	r, ctx := reconcilerFor(t, drLong)
	if _, err := r.Reconcile(ctx, reqFor(drLong)); err != nil {
		t.Fatalf("Reconcile(long name) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Vector — cluster not ready + truncated name
// ─────────────────────────────────────────────────────────────────────────────

func TestVector_ClusterNotReady(t *testing.T) {
	dr := newDR("vec-notready", "tenant-vecnotready", "vector")

	// Vector uses CloudNativePG clusters, readyInstances < instances means not ready
	pgGVK := schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "Cluster"}
	existingCluster := newUnstructuredCR(pgGVK, vectorClusterName(dr), dr.Spec.Tenant, map[string]interface{}{
		"readyInstances": int64(0), // not ready yet
	})

	r, ctx := reconcilerForWithUnstructured(t, dr, existingCluster)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(vector not ready) error = %v", err)
	}
}

func TestVector_LongNameTruncated(t *testing.T) {
	longName := "this-is-a-very-long-dataresource-name-exceeding-sixty-three-characters"
	drLong := newDR(longName, "tenant-vec", "vector")
	drShort := newDR("short", "t", "vector")

	// Long name → truncation branch
	name := vectorClusterName(drLong)
	if len(name) > 63 {
		t.Errorf("vectorClusterName too long: %d chars", len(name))
	}
	// Short name → non-truncation branch
	_ = vectorClusterName(drShort)

	r, ctx := reconcilerFor(t, drLong)
	if _, err := r.Reconcile(ctx, reqFor(drLong)); err != nil {
		t.Fatalf("Reconcile(long name) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Postgres — ready state (cluster exists with readyInstances >= instances)
// ─────────────────────────────────────────────────────────────────────────────

func TestPostgres_ClusterReady(t *testing.T) {
	dr := newDR("pg-rdy", "tenant-pgrdy", "postgres")

	pgGVK := schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "Cluster"}
	existingCluster := newUnstructuredCR(pgGVK, postgresClusterName(dr), dr.Spec.Tenant, nil)
	// Set readyInstances >= instances (default instances=1)
	_ = unstructured.SetNestedField(existingCluster.Object, int64(1), "status", "readyInstances")

	r, ctx := reconcilerForWithUnstructured(t, dr, existingCluster)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(postgres ready) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Postgres — DBLB ConfigMap update path (ConfigMap already exists)
// ─────────────────────────────────────────────────────────────────────────────

func TestPostgres_DblbConfigMapUpdate(t *testing.T) {
	dr := newDR("pg-dblb-upd", "tenant-pgdblbupd", "postgres")

	pgGVK := schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "Cluster"}
	existingCluster := newUnstructuredCR(pgGVK, postgresClusterName(dr), dr.Spec.Tenant, nil)
	_ = unstructured.SetNestedField(existingCluster.Object, int64(1), "status", "readyInstances")

	// Pre-create DBLB ConfigMap so the update path runs
	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dblbConfigMapName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Data: map[string]string{"dblb.conf": "old-config"},
	}

	r, ctx := reconcilerForWithUnstructured(t, dr, existingCluster)
	if err := r.Create(ctx, existingCM); err != nil {
		t.Fatalf("pre-create DBLB ConfigMap: %v", err)
	}
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(postgres dblb update) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// MariaDB — DBLB ConfigMap update path
// ─────────────────────────────────────────────────────────────────────────────

func TestMariaDB_DblbConfigMapUpdate(t *testing.T) {
	dr := newDR("mdb-dblb-upd", "tenant-mdbdblbupd", "mariadb")

	mariadbGVK := schema.GroupVersionKind{Group: "k8s.mariadb.com", Version: "v1alpha1", Kind: "MariaDB"}
	existingCluster := newUnstructuredCR(mariadbGVK, mariadbClusterName(dr), dr.Spec.Tenant, map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{
				"type":   "Ready",
				"status": "True",
			},
		},
	})

	// Pre-create DBLB ConfigMap so the update path runs
	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("dblb-mariadb-%s-%s", dr.Spec.Tenant, dr.Name),
			Namespace: dr.Spec.Tenant,
		},
		Data: map[string]string{"dblb.conf": "old-config"},
	}

	r, ctx := reconcilerForWithUnstructured(t, dr, existingCluster)
	if err := r.Create(ctx, existingCM); err != nil {
		t.Fatalf("pre-create MariaDB DBLB ConfigMap: %v", err)
	}
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(mariadb dblb update) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// MySQL — DBLB ConfigMap update path
// ─────────────────────────────────────────────────────────────────────────────

func TestMySQL_DblbConfigMapUpdate(t *testing.T) {
	dr := newDR("mysql-dblb-upd", "tenant-mysqldblbupd", "mysql")

	mysqlGVK := schema.GroupVersionKind{Group: "mysql.oracle.com", Version: "v2", Kind: "InnoDBCluster"}
	existingCluster := newUnstructuredCR(mysqlGVK, mysqlClusterName(dr), dr.Spec.Tenant, nil)
	_ = unstructured.SetNestedField(existingCluster.Object, "ONLINE", "status", "cluster", "status")

	// Pre-create DBLB ConfigMap so the update path runs
	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("dblb-mysql-%s-%s", dr.Spec.Tenant, dr.Name),
			Namespace: dr.Spec.Tenant,
		},
		Data: map[string]string{"dblb.conf": "old-config"},
	}

	r, ctx := reconcilerForWithUnstructured(t, dr, existingCluster)
	if err := r.Create(ctx, existingCM); err != nil {
		t.Fatalf("pre-create MySQL DBLB ConfigMap: %v", err)
	}
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(mysql dblb update) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Keyvalue — ConfigMap exists → update path (+ StatefulSet ready)
// ─────────────────────────────────────────────────────────────────────────────

func TestKeyvalue_ConfigMapExistsUpdate(t *testing.T) {
	dr := newDR("kv-cm-upd", "tenant-kvcmupd", "keyvalue")

	// Pre-create ConfigMap to trigger update path
	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      keyvalueConfigMapName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Data: map[string]string{"valkey.conf": "old-config"},
	}
	r, ctx := reconcilerFor(t, dr, existingCM)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(keyvalue CM update) error = %v", err)
	}
}

func TestKeyvalue_AllResourcesExist_ReadyState(t *testing.T) {
	dr := newDR("kv-all-rdy", "tenant-kvallrdy", "keyvalue")
	replicas := int32(1)

	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      keyvalueConfigMapName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Data: map[string]string{"valkey.conf": "existing"},
	}
	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      keyvalueServiceName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	existingSS := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      keyvalueStatefulSetName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Spec:   appsv1.StatefulSetSpec{Replicas: &replicas},
		Status: appsv1.StatefulSetStatus{ReadyReplicas: 1},
	}
	r, ctx := reconcilerFor(t, dr, existingCM, existingSvc, existingSS)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(keyvalue all resources ready) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Trino — ConfigMaps exist → update paths (coordinator + worker CM update)
// ─────────────────────────────────────────────────────────────────────────────

func TestTrino_ConfigMapsExist_UpdatePath(t *testing.T) {
	dr := newDR("trino-cm-upd", "tenant-trinocmupd", "warehouse/trino")

	// Pre-create both coordinator and worker ConfigMaps to trigger update paths
	coordCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoCoordinatorConfigMapName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Data: map[string]string{"coordinator.properties": "old"},
	}
	workerCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoWorkerConfigMapName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Data: map[string]string{"worker.properties": "old"},
	}
	r, ctx := reconcilerFor(t, dr, coordCM, workerCM)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(trino CM update paths) error = %v", err)
	}
}

func TestTrino_CoordinatorReadyWorkerReady_ServiceExists(t *testing.T) {
	dr := newDR("trino-all-rdy", "tenant-trinoallrdy", "warehouse/trino")
	replicas := int32(2)

	coordCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoCoordinatorConfigMapName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	workerCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoWorkerConfigMapName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	coordDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoCoordinatorName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Spec:   appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{AvailableReplicas: 1},
	}
	workerDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoWorkerName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Spec:   appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{AvailableReplicas: 2},
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoServiceName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}
	r, ctx := reconcilerFor(t, dr, coordCM, workerCM, coordDeploy, workerDeploy, svc)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(trino coordinator+worker ready, service exists) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// FerretDB — PG cluster not ready (provisioning wait) + deployment exists not ready
// ─────────────────────────────────────────────────────────────────────────────

func TestFerretDB_PGClusterExistsNotReady(t *testing.T) {
	dr := newDR("ferret-pg-notready", "tenant-ferretpgnr", "rockfs")

	pgGVK := schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "Cluster"}
	existingPG := newUnstructuredCR(pgGVK, ferretdbPGClusterName(dr), dr.Spec.Tenant, nil)
	// readyInstances = 0 (not ready)
	_ = unstructured.SetNestedField(existingPG.Object, int64(0), "status", "readyInstances")

	r, ctx := reconcilerForWithUnstructured(t, dr, existingPG)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(ferretdb pg not ready) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// MariaDB — ready-state path with Secret already exists for delete
// ─────────────────────────────────────────────────────────────────────────────

func TestMariaDB_Delete_WithClusterAndSecret(t *testing.T) {
	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "mdb-del-sec",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{Type: "mariadb", Tenant: "tenant-mdbdelsec"},
	}

	mariadbGVK := schema.GroupVersionKind{Group: "k8s.mariadb.com", Version: "v1alpha1", Kind: "MariaDB"}
	existingCluster := newUnstructuredCR(mariadbGVK, mariadbClusterName(dr), dr.Spec.Tenant, nil)

	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("mariadb-%s-%s-root", dr.Spec.Tenant, dr.Name),
			Namespace: dr.Spec.Tenant,
		},
	}

	r, ctx := reconcilerForWithUnstructured(t, dr, existingCluster)
	if err := r.Create(ctx, existingSecret); err != nil {
		t.Fatalf("pre-create MariaDB secret: %v", err)
	}
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete mariadb with secret) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// MySQL — delete with Secret pre-created
// ─────────────────────────────────────────────────────────────────────────────

func TestMySQL_Delete_WithClusterAndSecret(t *testing.T) {
	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "mysql-del-sec",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{Type: "mysql", Tenant: "tenant-mysqldelsec"},
	}

	mysqlGVK := schema.GroupVersionKind{Group: "mysql.oracle.com", Version: "v2", Kind: "InnoDBCluster"}
	existingCluster := newUnstructuredCR(mysqlGVK, mysqlClusterName(dr), dr.Spec.Tenant, nil)

	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("mysql-%s-%s-root", dr.Spec.Tenant, dr.Name),
			Namespace: dr.Spec.Tenant,
		},
	}

	r, ctx := reconcilerForWithUnstructured(t, dr, existingCluster)
	if err := r.Create(ctx, existingSecret); err != nil {
		t.Fatalf("pre-create MySQL secret: %v", err)
	}
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete mysql with secret) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Iceberg — PG cluster exists (already created path in reconcileIcebergPostgres)
// ─────────────────────────────────────────────────────────────────────────────

func TestIceberg_PGClusterExists_DeploymentAndServiceExist_NotReady(t *testing.T) {
	dr := newDR("ice-pg-exist", "tenant-icepgexist", "lakehouse/iceberg")

	pgGVK := schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "Cluster"}
	existingPG := newUnstructuredCR(pgGVK, icebergPGClusterName(dr), dr.Spec.Tenant, nil)

	// Deployment exists but not ready (AvailableReplicas = 0)
	existingDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      icebergRestName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Status: appsv1.DeploymentStatus{AvailableReplicas: 0},
	}
	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      icebergRestName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}

	r, ctx := reconcilerForWithUnstructured(t, dr, existingPG)
	if err := r.Create(ctx, existingDeploy); err != nil {
		t.Fatalf("pre-create iceberg deployment: %v", err)
	}
	if err := r.Create(ctx, existingSvc); err != nil {
		t.Fatalf("pre-create iceberg service: %v", err)
	}
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(iceberg pg exists, deploy not ready) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Vector — PG cluster exists + ready state
// ─────────────────────────────────────────────────────────────────────────────

func TestVector_PGClusterReady(t *testing.T) {
	dr := newDR("vec-pg-rdy", "tenant-vecpgrdy", "vector")

	pgGVK := schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "Cluster"}
	existingCluster := newUnstructuredCR(pgGVK, vectorClusterName(dr), dr.Spec.Tenant, nil)
	_ = unstructured.SetNestedField(existingCluster.Object, int64(1), "status", "readyInstances")

	r, ctx := reconcilerForWithUnstructured(t, dr, existingCluster)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(vector pg ready) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// OpenSearch — cluster exists + ready (RUNNING) state
// ─────────────────────────────────────────────────────────────────────────────

func TestOpenSearch_ClusterRunning(t *testing.T) {
	dr := newDR("os-running", "tenant-osrun", "search")

	osGVK := schema.GroupVersionKind{Group: "opensearch.opster.io", Version: "v1", Kind: "OpenSearchCluster"}
	existingCluster := newUnstructuredCR(osGVK, opensearchClusterName(dr), dr.Spec.Tenant, nil)
	_ = unstructured.SetNestedField(existingCluster.Object, "RUNNING", "status", "phase")

	r, ctx := reconcilerForWithUnstructured(t, dr, existingCluster)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(opensearch running) error = %v", err)
	}
}

func TestOpenSearch_ClusterNotRunning(t *testing.T) {
	dr := newDR("os-pending", "tenant-ospend", "search")

	osGVK := schema.GroupVersionKind{Group: "opensearch.opster.io", Version: "v1", Kind: "OpenSearchCluster"}
	existingCluster := newUnstructuredCR(osGVK, opensearchClusterName(dr), dr.Spec.Tenant, nil)
	_ = unstructured.SetNestedField(existingCluster.Object, "PENDING", "status", "phase")

	r, ctx := reconcilerForWithUnstructured(t, dr, existingCluster)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(opensearch not running) error = %v", err)
	}
}

func TestOpenSearch_Delete_WithExistingCluster(t *testing.T) {
	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "os-del-exist",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{Type: "search", Tenant: "tenant-osdelexist"},
	}

	osGVK := schema.GroupVersionKind{Group: "opensearch.opster.io", Version: "v1", Kind: "OpenSearchCluster"}
	existingCluster := newUnstructuredCR(osGVK, opensearchClusterName(dr), dr.Spec.Tenant, nil)

	r, ctx := reconcilerForWithUnstructured(t, dr, existingCluster)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete opensearch) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Clickhouse — cluster ready (Completed) state + delete with existing cluster
// ─────────────────────────────────────────────────────────────────────────────

func TestClickhouse_ClusterCompleted(t *testing.T) {
	dr := newDR("chi-complete", "tenant-chicomplete", "clickhouse")

	chiGVK := schema.GroupVersionKind{Group: "clickhouse.altinity.com", Version: "v1", Kind: "ClickHouseInstallation"}
	existingCluster := newUnstructuredCR(chiGVK, clickhouseClusterName(dr), dr.Spec.Tenant, nil)
	_ = unstructured.SetNestedField(existingCluster.Object, "Completed", "status", "status")

	r, ctx := reconcilerForWithUnstructured(t, dr, existingCluster)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(clickhouse completed) error = %v", err)
	}
}

func TestClickhouse_Delete_WithExistingCluster(t *testing.T) {
	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "chi-del-exist",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{Type: "clickhouse", Tenant: "tenant-chidelexist"},
	}

	chiGVK := schema.GroupVersionKind{Group: "clickhouse.altinity.com", Version: "v1", Kind: "ClickHouseInstallation"}
	existingCluster := newUnstructuredCR(chiGVK, clickhouseClusterName(dr), dr.Spec.Tenant, nil)

	r, ctx := reconcilerForWithUnstructured(t, dr, existingCluster)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete clickhouse) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Kafka — cluster ready state + delete with existing cluster
// ─────────────────────────────────────────────────────────────────────────────

func TestKafka_ClusterReady(t *testing.T) {
	dr := newDR("kafka-rdy", "tenant-kafkrdy", "kafka")

	kafkaGVK := schema.GroupVersionKind{Group: "kafka.strimzi.io", Version: "v1beta2", Kind: "Kafka"}
	existingCluster := newUnstructuredCR(kafkaGVK, kafkaClusterName(dr), dr.Spec.Tenant, nil)
	_ = unstructured.SetNestedSlice(existingCluster.Object, []interface{}{
		map[string]interface{}{
			"type":   "Ready",
			"status": "True",
		},
	}, "status", "conditions")

	r, ctx := reconcilerForWithUnstructured(t, dr, existingCluster)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(kafka ready) error = %v", err)
	}
}

func TestKafka_Delete_WithExistingCluster(t *testing.T) {
	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "kafka-del-exist",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{Type: "kafka", Tenant: "tenant-kafkadelexist"},
	}

	kafkaGVK := schema.GroupVersionKind{Group: "kafka.strimzi.io", Version: "v1beta2", Kind: "Kafka"}
	existingCluster := newUnstructuredCR(kafkaGVK, kafkaClusterName(dr), dr.Spec.Tenant, nil)

	r, ctx := reconcilerForWithUnstructured(t, dr, existingCluster)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(delete kafka) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Error-injection helpers — uses controller-runtime interceptor to make
// Delete/Create/Update fail so logger.Error+return err branches are covered.
// ─────────────────────────────────────────────────────────────────────────────

// reconcilerWithDeleteError returns a reconciler whose fake client returns errInject
// on the first Delete call whose object name matches targetName.
func reconcilerWithDeleteError(t *testing.T, errInject error, targetName string, objects ...interface{}) (*DataResourceReconciler, context.Context) {
	t.Helper()
	scheme := newTestScheme(t)
	builder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&nestv1.DataResource{})
	for _, raw := range objects {
		switch v := raw.(type) {
		case *nestv1.DataResource:
			builder = builder.WithObjects(v)
		case *corev1.ConfigMap:
			builder = builder.WithObjects(v)
		case *corev1.Service:
			builder = builder.WithObjects(v)
		case *corev1.Secret:
			builder = builder.WithObjects(v)
		case *appsv1.Deployment:
			builder = builder.WithObjects(v)
		case *appsv1.StatefulSet:
			builder = builder.WithObjects(v)
		}
	}
	baseClient := builder.Build()
	called := false
	wrappedClient := interceptor.NewClient(baseClient, interceptor.Funcs{
		Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			if !called && obj.GetName() == targetName {
				called = true
				return errInject
			}
			return c.Delete(ctx, obj, opts...)
		},
	})
	r := &DataResourceReconciler{Client: wrappedClient, Scheme: scheme}
	return r, context.Background()
}

// reconcilerWithDeleteErrorOnKind injects an error on the first Delete of an object
// whose name matches targetName AND whose GVK Kind matches targetKind.
// This is needed when service and statefulset share the same name.
func reconcilerWithDeleteErrorOnKind(t *testing.T, errInject error, targetName, targetKind string, objects ...interface{}) (*DataResourceReconciler, context.Context) {
	t.Helper()
	scheme := newTestScheme(t)
	builder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&nestv1.DataResource{})
	for _, raw := range objects {
		switch v := raw.(type) {
		case *nestv1.DataResource:
			builder = builder.WithObjects(v)
		case *corev1.ConfigMap:
			builder = builder.WithObjects(v)
		case *corev1.Service:
			builder = builder.WithObjects(v)
		case *corev1.Secret:
			builder = builder.WithObjects(v)
		case *appsv1.Deployment:
			builder = builder.WithObjects(v)
		case *appsv1.StatefulSet:
			builder = builder.WithObjects(v)
		}
	}
	baseClient := builder.Build()
	called := false
	wrappedClient := interceptor.NewClient(baseClient, interceptor.Funcs{
		Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			// Determine kind by type assertion on concrete Go types
			var kind string
			switch obj.(type) {
			case *corev1.Service:
				kind = "Service"
			case *appsv1.StatefulSet:
				kind = "StatefulSet"
			case *appsv1.Deployment:
				kind = "Deployment"
			case *corev1.ConfigMap:
				kind = "ConfigMap"
			case *corev1.Secret:
				kind = "Secret"
			default:
				// unstructured: use GVK embedded in object
				kind = obj.GetObjectKind().GroupVersionKind().Kind
			}
			if !called && obj.GetName() == targetName && kind == targetKind {
				called = true
				return errInject
			}
			return c.Delete(ctx, obj, opts...)
		},
	})
	r := &DataResourceReconciler{Client: wrappedClient, Scheme: scheme}
	return r, context.Background()
}

// reconcilerWithUpdateError injects an error on the first Update of an object
// whose name matches targetName.
func reconcilerWithUpdateError(t *testing.T, errInject error, targetName string, objects ...interface{}) (*DataResourceReconciler, context.Context) {
	t.Helper()
	scheme := newTestScheme(t)
	builder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&nestv1.DataResource{})
	for _, raw := range objects {
		switch v := raw.(type) {
		case *nestv1.DataResource:
			builder = builder.WithObjects(v)
		case *corev1.ConfigMap:
			builder = builder.WithObjects(v)
		case *corev1.Service:
			builder = builder.WithObjects(v)
		case *corev1.Secret:
			builder = builder.WithObjects(v)
		case *appsv1.Deployment:
			builder = builder.WithObjects(v)
		case *appsv1.StatefulSet:
			builder = builder.WithObjects(v)
		}
	}
	baseClient := builder.Build()
	called := false
	wrappedClient := interceptor.NewClient(baseClient, interceptor.Funcs{
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			if !called && obj.GetName() == targetName {
				called = true
				return errInject
			}
			return c.Update(ctx, obj, opts...)
		},
	})
	r := &DataResourceReconciler{Client: wrappedClient, Scheme: scheme}
	return r, context.Background()
}

// ─────────────────────────────────────────────────────────────────────────────
// Trino — custom replicas spec path
// ─────────────────────────────────────────────────────────────────────────────

func TestTrino_CustomReplicas(t *testing.T) {
	dr := newDR("trino-custom-replicas", "tenant-trinocustom", "warehouse/trino")
	dr.Spec.Replicas = &nestv1.ReplicaConfig{
		Write: &nestv1.ReplicaCountSpec{Default: 4},
	}
	r, ctx := reconcilerFor(t, dr)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err != nil {
		t.Fatalf("Reconcile(trino custom replicas) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// DarkDrive controller — state machine tests
// ─────────────────────────────────────────────────────────────────────────────

func newDarkDriveReconciler(t *testing.T, objects ...client.Object) (*DarkDriveReconciler, context.Context) {
	t.Helper()
	scheme := newTestScheme(t)
	builder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&nestv1.DarkDrive{})
	for _, obj := range objects {
		builder = builder.WithObjects(obj)
	}
	fakeClient := builder.Build()
	r := &DarkDriveReconciler{Client: fakeClient, Scheme: scheme}
	return r, context.Background()
}

func ddReq(dd *nestv1.DarkDrive) ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{Name: dd.Name, Namespace: dd.Namespace}}
}

func TestDarkDrive_NewResource_InitToDiscovered(t *testing.T) {
	dd := &nestv1.DarkDrive{
		ObjectMeta: metav1.ObjectMeta{Name: "nvme0n1", Namespace: "default"},
		Spec: nestv1.DarkDriveSpec{
			Node:   "node1",
			Device: "/dev/nvme0n1",
			Size:   "1.92TB",
			Class:  "nvme-hot",
		},
	}
	r, ctx := newDarkDriveReconciler(t, dd)
	if _, err := r.Reconcile(ctx, ddReq(dd)); err != nil {
		t.Fatalf("Reconcile(new DD) error = %v", err)
	}
}

func TestDarkDrive_Discovered_ToAwaitingApproval(t *testing.T) {
	dd := &nestv1.DarkDrive{
		ObjectMeta: metav1.ObjectMeta{Name: "nvme0n2", Namespace: "default"},
		Spec: nestv1.DarkDriveSpec{
			Node:   "node1",
			Device: "/dev/nvme0n2",
			Size:   "1.92TB",
			Class:  "nvme-hot",
		},
		Status: nestv1.DarkDriveStatus{
			State: nestv1.DarkDriveDiscovered,
		},
	}
	r, ctx := newDarkDriveReconciler(t, dd)
	if _, err := r.Reconcile(ctx, ddReq(dd)); err != nil {
		t.Fatalf("Reconcile(discovered) error = %v", err)
	}
}

func TestDarkDrive_AwaitingApproval_NoOp(t *testing.T) {
	dd := &nestv1.DarkDrive{
		ObjectMeta: metav1.ObjectMeta{Name: "nvme0n3", Namespace: "default"},
		Spec: nestv1.DarkDriveSpec{
			Node:  "node1",
			Class: "nvme-hot",
		},
		Status: nestv1.DarkDriveStatus{
			State: nestv1.DarkDriveAwaitingApproval,
		},
	}
	r, ctx := newDarkDriveReconciler(t, dd)
	if _, err := r.Reconcile(ctx, ddReq(dd)); err != nil {
		t.Fatalf("Reconcile(awaiting approval) error = %v", err)
	}
}

func TestDarkDrive_Approved_ForeignFSBlocked(t *testing.T) {
	dd := &nestv1.DarkDrive{
		ObjectMeta: metav1.ObjectMeta{Name: "nvme0n4", Namespace: "default"},
		Spec: nestv1.DarkDriveSpec{
			Node:           "node1",
			Class:          "nvme-hot",
			Signature:      "foreign-fs:ext4",
			EraseConfirmed: false,
			HardwarePool:   "pool-nvme",
		},
		Status: nestv1.DarkDriveStatus{
			State: nestv1.DarkDriveApproved,
		},
	}
	r, ctx := newDarkDriveReconciler(t, dd)
	if _, err := r.Reconcile(ctx, ddReq(dd)); err != nil {
		t.Fatalf("Reconcile(approved foreign-fs blocked) error = %v", err)
	}
}

func TestDarkDrive_Approved_NoHardwarePool(t *testing.T) {
	dd := &nestv1.DarkDrive{
		ObjectMeta: metav1.ObjectMeta{Name: "nvme0n5", Namespace: "default"},
		Spec: nestv1.DarkDriveSpec{
			Node:  "node1",
			Class: "nvme-hot",
			// HardwarePool deliberately empty
		},
		Status: nestv1.DarkDriveStatus{
			State: nestv1.DarkDriveApproved,
		},
	}
	r, ctx := newDarkDriveReconciler(t, dd)
	if _, err := r.Reconcile(ctx, ddReq(dd)); err != nil {
		t.Fatalf("Reconcile(approved no pool) error = %v", err)
	}
}

func TestDarkDrive_Approved_Adopted(t *testing.T) {
	dd := &nestv1.DarkDrive{
		ObjectMeta: metav1.ObjectMeta{Name: "nvme0n6", Namespace: "default"},
		Spec: nestv1.DarkDriveSpec{
			Node:         "node1",
			Class:        "nvme-hot",
			HardwarePool: "pool-nvme",
		},
		Status: nestv1.DarkDriveStatus{
			State: nestv1.DarkDriveApproved,
		},
	}
	r, ctx := newDarkDriveReconciler(t, dd)
	if _, err := r.Reconcile(ctx, ddReq(dd)); err != nil {
		t.Fatalf("Reconcile(approved→adopted) error = %v", err)
	}
}

func TestDarkDrive_Rejected_Terminal(t *testing.T) {
	dd := &nestv1.DarkDrive{
		ObjectMeta: metav1.ObjectMeta{Name: "nvme0n7", Namespace: "default"},
		Spec:       nestv1.DarkDriveSpec{Node: "node1", Class: "nvme-hot"},
		Status:     nestv1.DarkDriveStatus{State: nestv1.DarkDriveRejected},
	}
	r, ctx := newDarkDriveReconciler(t, dd)
	if _, err := r.Reconcile(ctx, ddReq(dd)); err != nil {
		t.Fatalf("Reconcile(rejected) error = %v", err)
	}
}

func TestDarkDrive_Adopted_Terminal(t *testing.T) {
	dd := &nestv1.DarkDrive{
		ObjectMeta: metav1.ObjectMeta{Name: "nvme0n8", Namespace: "default"},
		Spec:       nestv1.DarkDriveSpec{Node: "node1", Class: "nvme-hot"},
		Status:     nestv1.DarkDriveStatus{State: nestv1.DarkDriveAdopted},
	}
	r, ctx := newDarkDriveReconciler(t, dd)
	if _, err := r.Reconcile(ctx, ddReq(dd)); err != nil {
		t.Fatalf("Reconcile(adopted) error = %v", err)
	}
}

func TestDarkDrive_UnknownState(t *testing.T) {
	dd := &nestv1.DarkDrive{
		ObjectMeta: metav1.ObjectMeta{Name: "nvme0n9", Namespace: "default"},
		Spec:       nestv1.DarkDriveSpec{Node: "node1", Class: "nvme-hot"},
		Status:     nestv1.DarkDriveStatus{State: "UnknownState"},
	}
	r, ctx := newDarkDriveReconciler(t, dd)
	if _, err := r.Reconcile(ctx, ddReq(dd)); err != nil {
		t.Fatalf("Reconcile(unknown state) error = %v", err)
	}
}

func TestDarkDrive_NotFound_NoError(t *testing.T) {
	r, ctx := newDarkDriveReconciler(t)
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: "default"}}
	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("Reconcile(not found) error = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Update error path tests — inject Update errors to cover update error branches
// ─────────────────────────────────────────────────────────────────────────────

func TestKeyvalue_ConfigMap_UpdateError(t *testing.T) {
	dr := newDR("kv-cm-upd-err", "tenant-kvcmupdaterr", "keyvalue")

	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      keyvalueConfigMapName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Data: map[string]string{"old": "data"},
	}

	injectedErr := fmt.Errorf("injected cm update error")
	r, ctx := reconcilerWithUpdateError(t, injectedErr, keyvalueConfigMapName(dr), dr, existingCM)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err == nil {
		t.Fatal("Reconcile(keyvalue CM update error) expected error, got nil")
	}
}

func TestTrino_CoordCM_UpdateError(t *testing.T) {
	dr := newDR("trino-cm-upd-err", "tenant-trinocmupdaterr", "warehouse/trino")

	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoCoordinatorConfigMapName(dr),
			Namespace: dr.Spec.Tenant,
		},
		Data: map[string]string{"old": "data"},
	}

	injectedErr := fmt.Errorf("injected trino coord CM update error")
	r, ctx := reconcilerWithUpdateError(t, injectedErr, trinoCoordinatorConfigMapName(dr), dr, existingCM)
	if _, err := r.Reconcile(ctx, reqFor(dr)); err == nil {
		t.Fatal("Reconcile(trino coord CM update error) expected error, got nil")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Delete error path tests — inject Delete errors to cover logger.Error+return err
// ─────────────────────────────────────────────────────────────────────────────

func TestKeyvalue_Delete_StatefulSetDeleteError(t *testing.T) {
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "kv-del-err", Namespace: "default"},
		Spec:       nestv1.DataResourceSpec{Type: "keyvalue", Tenant: "tenant-kvdelerr"},
	}

	existingSS := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      keyvalueStatefulSetName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}

	injectedErr := fmt.Errorf("injected delete error")
	r, ctx := reconcilerWithDeleteError(t, injectedErr, keyvalueStatefulSetName(dr), dr, existingSS)
	if err := r.reconcileKeyvalueDelete(ctx, dr); err == nil {
		t.Fatal("reconcileKeyvalueDelete(SS delete error) expected error, got nil")
	}
}

func TestKeyvalue_Delete_ServiceDeleteError(t *testing.T) {
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "kv-del-svc-err", Namespace: "default"},
		Spec:       nestv1.DataResourceSpec{Type: "keyvalue", Tenant: "tenant-kvdelsvcerr"},
	}

	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      keyvalueServiceName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}

	injectedErr := fmt.Errorf("injected svc delete error")
	r, ctx := reconcilerWithDeleteError(t, injectedErr, keyvalueServiceName(dr), dr, existingSvc)
	if err := r.reconcileKeyvalueDelete(ctx, dr); err == nil {
		t.Fatal("reconcileKeyvalueDelete(Svc delete error) expected error, got nil")
	}
}

func TestKeyvalue_Delete_ConfigMapDeleteError(t *testing.T) {
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "kv-del-cm-err", Namespace: "default"},
		Spec:       nestv1.DataResourceSpec{Type: "keyvalue", Tenant: "tenant-kvdelcmerr"},
	}

	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      keyvalueConfigMapName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}

	injectedErr := fmt.Errorf("injected cm delete error")
	r, ctx := reconcilerWithDeleteError(t, injectedErr, keyvalueConfigMapName(dr), dr, existingCM)
	if err := r.reconcileKeyvalueDelete(ctx, dr); err == nil {
		t.Fatal("reconcileKeyvalueDelete(CM delete error) expected error, got nil")
	}
}

func TestTimeseries_Delete_StatefulSetDeleteError(t *testing.T) {
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "ts-del-ss-err", Namespace: "default"},
		Spec:       nestv1.DataResourceSpec{Type: "timeseries", Tenant: "tenant-tsdelsserr"},
	}

	existingSS := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      timeseriesStatefulSetName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}

	injectedErr := fmt.Errorf("injected ts SS delete error")
	r, ctx := reconcilerWithDeleteError(t, injectedErr, timeseriesStatefulSetName(dr), dr, existingSS)
	if err := r.reconcileTimeseriesDelete(ctx, dr); err == nil {
		t.Fatal("reconcileTimeseriesDelete(SS delete error) expected error, got nil")
	}
}

func TestTimeseries_Delete_ServiceDeleteError(t *testing.T) {
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "ts-del-svc-err", Namespace: "default"},
		Spec:       nestv1.DataResourceSpec{Type: "timeseries", Tenant: "tenant-tsdelsvcer"},
	}

	// timeseries Service and StatefulSet share the same name, so use kind-based interceptor
	// to target the Service delete specifically (StatefulSet delete comes first and must succeed).
	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      timeseriesServiceName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}

	injectedErr := fmt.Errorf("injected ts svc delete error")
	r, ctx := reconcilerWithDeleteErrorOnKind(t, injectedErr, timeseriesServiceName(dr), "Service", dr, existingSvc)
	if err := r.reconcileTimeseriesDelete(ctx, dr); err == nil {
		t.Fatal("reconcileTimeseriesDelete(Svc delete error) expected error, got nil")
	}
}

func TestFerretDB_Delete_DeploymentDeleteError(t *testing.T) {
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "ferret-del-err", Namespace: "default"},
		Spec:       nestv1.DataResourceSpec{Type: "rockfs", Tenant: "tenant-ferretdelerr"},
	}

	existingDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ferretdbDeploymentName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}

	injectedErr := fmt.Errorf("injected ferretdb deploy delete error")
	r, ctx := reconcilerWithDeleteError(t, injectedErr, ferretdbDeploymentName(dr), dr, existingDeploy)
	// reconcileFerretDBDelete propagates the delete error internally
	if err := r.reconcileFerretDBDelete(ctx, dr); err == nil {
		t.Fatal("reconcileFerretDBDelete(deploy delete error) expected error, got nil")
	}
}

func TestFerretDB_Delete_ServiceDeleteError(t *testing.T) {
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "ferret-del-svc-err", Namespace: "default"},
		Spec:       nestv1.DataResourceSpec{Type: "rockfs", Tenant: "tenant-ferretdelsvcerr"},
	}

	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ferretdbDeploymentName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}

	injectedErr := fmt.Errorf("injected ferretdb svc delete error")
	// FerretDB deploy and service share the same name — use kind-based interceptor for Service
	r, ctx := reconcilerWithDeleteErrorOnKind(t, injectedErr, ferretdbDeploymentName(dr), "Service", dr, existingSvc)
	if err := r.reconcileFerretDBDelete(ctx, dr); err == nil {
		t.Fatal("reconcileFerretDBDelete(svc delete error) expected error, got nil")
	}
}

func TestMariaDB_Delete_ClusterDeleteError(t *testing.T) {
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "mariadb-del-err", Namespace: "default"},
		Spec:       nestv1.DataResourceSpec{Type: "mariadb", Tenant: "tenant-mariadbdelerr"},
	}

	injectedErr := fmt.Errorf("injected mariadb cluster delete error")
	r, ctx := reconcilerWithDeleteError(t, injectedErr, mariadbClusterName(dr))
	if err := r.reconcileMariaDBDelete(ctx, dr); err == nil {
		t.Fatal("reconcileMariaDBDelete(cluster delete error) expected error, got nil")
	}
}

func TestMariaDB_Delete_SecretDeleteError(t *testing.T) {
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "mariadb-del-sec-err", Namespace: "default"},
		Spec:       nestv1.DataResourceSpec{Type: "mariadb", Tenant: "tenant-mariadbdelsecerr"},
	}

	secretName := fmt.Sprintf("mariadb-%s-%s-root", dr.Spec.Tenant, dr.Name)
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: dr.Spec.Tenant},
	}

	injectedErr := fmt.Errorf("injected mariadb secret delete error")
	// Use kind-based interceptor: cluster is unstructured (kind=MariaDB), secret is corev1.Secret
	r, ctx := reconcilerWithDeleteErrorOnKind(t, injectedErr, secretName, "Secret", dr, existingSecret)
	if err := r.reconcileMariaDBDelete(ctx, dr); err == nil {
		t.Fatal("reconcileMariaDBDelete(secret delete error) expected error, got nil")
	}
}

func TestMySQL_Delete_ClusterDeleteError(t *testing.T) {
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "mysql-del-err", Namespace: "default"},
		Spec:       nestv1.DataResourceSpec{Type: "mysql", Tenant: "tenant-mysqldelerr"},
	}

	injectedErr := fmt.Errorf("injected mysql cluster delete error")
	r, ctx := reconcilerWithDeleteError(t, injectedErr, mysqlClusterName(dr))
	if err := r.reconcileMySQLDelete(ctx, dr); err == nil {
		t.Fatal("reconcileMySQLDelete(cluster delete error) expected error, got nil")
	}
}

func TestMySQL_Delete_SecretDeleteError(t *testing.T) {
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "mysql-del-sec-err", Namespace: "default"},
		Spec:       nestv1.DataResourceSpec{Type: "mysql", Tenant: "tenant-mysqldelsecerr"},
	}

	secretName := fmt.Sprintf("mysql-%s-%s-root", dr.Spec.Tenant, dr.Name)
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: dr.Spec.Tenant},
	}

	injectedErr := fmt.Errorf("injected mysql secret delete error")
	r, ctx := reconcilerWithDeleteErrorOnKind(t, injectedErr, secretName, "Secret", dr, existingSecret)
	if err := r.reconcileMySQLDelete(ctx, dr); err == nil {
		t.Fatal("reconcileMySQLDelete(secret delete error) expected error, got nil")
	}
}

func TestVector_Delete_ClusterDeleteError(t *testing.T) {
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "vec-del-err", Namespace: "default"},
		Spec:       nestv1.DataResourceSpec{Type: "vector", Tenant: "tenant-vecdelerr"},
	}

	injectedErr := fmt.Errorf("injected vector cluster delete error")
	r, ctx := reconcilerWithDeleteError(t, injectedErr, vectorClusterName(dr))
	if err := r.reconcileVectorDelete(ctx, dr); err == nil {
		t.Fatal("reconcileVectorDelete(cluster delete error) expected error, got nil")
	}
}

func TestIceberg_Delete_DeploymentDeleteError(t *testing.T) {
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "iceberg-del-dep-err", Namespace: "default"},
		Spec:       nestv1.DataResourceSpec{Type: "lakehouse/iceberg", Tenant: "tenant-icebergdeldep"},
	}

	existingDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      icebergRestName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}

	injectedErr := fmt.Errorf("injected iceberg deploy delete error")
	r, ctx := reconcilerWithDeleteErrorOnKind(t, injectedErr, icebergRestName(dr), "Deployment", dr, existingDeploy)
	if err := r.reconcileIcebergDelete(ctx, dr); err == nil {
		t.Fatal("reconcileIcebergDelete(deploy delete error) expected error, got nil")
	}
}

func TestIceberg_Delete_ServiceDeleteError(t *testing.T) {
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "iceberg-del-svc-err", Namespace: "default"},
		Spec:       nestv1.DataResourceSpec{Type: "lakehouse/iceberg", Tenant: "tenant-icebergdelsvc"},
	}

	// Iceberg deploy and service share the same name (icebergRestName)
	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      icebergRestName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}

	injectedErr := fmt.Errorf("injected iceberg svc delete error")
	r, ctx := reconcilerWithDeleteErrorOnKind(t, injectedErr, icebergRestName(dr), "Service", dr, existingSvc)
	if err := r.reconcileIcebergDelete(ctx, dr); err == nil {
		t.Fatal("reconcileIcebergDelete(svc delete error) expected error, got nil")
	}
}

func TestIceberg_Delete_PGClusterDeleteError(t *testing.T) {
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "iceberg-del-pg-err", Namespace: "default"},
		Spec:       nestv1.DataResourceSpec{Type: "lakehouse/iceberg", Tenant: "tenant-icebergdelpg"},
	}

	injectedErr := fmt.Errorf("injected iceberg pg cluster delete error")
	r, ctx := reconcilerWithDeleteError(t, injectedErr, icebergPGClusterName(dr))
	if err := r.reconcileIcebergDelete(ctx, dr); err == nil {
		t.Fatal("reconcileIcebergDelete(PG cluster delete error) expected error, got nil")
	}
}

func TestFerretDB_Delete_PGClusterDeleteError(t *testing.T) {
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "ferret-del-pg-err", Namespace: "default"},
		Spec:       nestv1.DataResourceSpec{Type: "rockfs", Tenant: "tenant-ferretdelpgerr"},
	}

	injectedErr := fmt.Errorf("injected ferretdb pg cluster delete error")
	r, ctx := reconcilerWithDeleteError(t, injectedErr, ferretdbPGClusterName(dr))
	if err := r.reconcileFerretDBDelete(ctx, dr); err == nil {
		t.Fatal("reconcileFerretDBDelete(PG cluster delete error) expected error, got nil")
	}
}

func TestKafka_Delete_ClusterDeleteError(t *testing.T) {
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "kafka-del-err", Namespace: "default"},
		Spec:       nestv1.DataResourceSpec{Type: "kafka", Tenant: "tenant-kafkadelerr"},
	}

	injectedErr := fmt.Errorf("injected kafka cluster delete error")
	r, ctx := reconcilerWithDeleteError(t, injectedErr, kafkaClusterName(dr))
	if err := r.reconcileKafkaDelete(ctx, dr); err == nil {
		t.Fatal("reconcileKafkaDelete(cluster delete error) expected error, got nil")
	}
}

func TestClickhouse_Delete_ClusterDeleteError(t *testing.T) {
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "chi-del-err", Namespace: "default"},
		Spec:       nestv1.DataResourceSpec{Type: "clickhouse", Tenant: "tenant-chidelerr"},
	}

	injectedErr := fmt.Errorf("injected clickhouse cluster delete error")
	r, ctx := reconcilerWithDeleteError(t, injectedErr, clickhouseClusterName(dr))
	if err := r.reconcileClickhouseDelete(ctx, dr); err == nil {
		t.Fatal("reconcileClickhouseDelete(cluster delete error) expected error, got nil")
	}
}

func TestOpenSearch_Delete_ClusterDeleteError(t *testing.T) {
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "os-del-err", Namespace: "default"},
		Spec:       nestv1.DataResourceSpec{Type: "search", Tenant: "tenant-osdelerr"},
	}

	injectedErr := fmt.Errorf("injected opensearch cluster delete error")
	r, ctx := reconcilerWithDeleteError(t, injectedErr, opensearchClusterName(dr))
	if err := r.reconcileOpenSearchDelete(ctx, dr); err == nil {
		t.Fatal("reconcileOpenSearchDelete(cluster delete error) expected error, got nil")
	}
}

func TestTrino_Delete_CoordinatorDeployDeleteError(t *testing.T) {
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "trino-del-coord-err", Namespace: "default"},
		Spec:       nestv1.DataResourceSpec{Type: "warehouse/trino", Tenant: "tenant-trinodelcerr"},
	}

	existingDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoCoordinatorName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}

	injectedErr := fmt.Errorf("injected coordinator deploy delete error")
	r, ctx := reconcilerWithDeleteError(t, injectedErr, trinoCoordinatorName(dr), dr, existingDeploy)
	if err := r.reconcileTrinoDelete(ctx, dr); err == nil {
		t.Fatal("reconcileTrinoDelete(coordinator deploy delete error) expected error, got nil")
	}
}

func TestTrino_Delete_WorkerDeployDeleteError(t *testing.T) {
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "trino-del-work-err", Namespace: "default"},
		Spec:       nestv1.DataResourceSpec{Type: "warehouse/trino", Tenant: "tenant-trinodelwerr"},
	}

	existingDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoWorkerName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}

	injectedErr := fmt.Errorf("injected worker deploy delete error")
	r, ctx := reconcilerWithDeleteError(t, injectedErr, trinoWorkerName(dr), dr, existingDeploy)
	if err := r.reconcileTrinoDelete(ctx, dr); err == nil {
		t.Fatal("reconcileTrinoDelete(worker deploy delete error) expected error, got nil")
	}
}

func TestTrino_Delete_ServiceDeleteError(t *testing.T) {
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "trino-del-svc-err", Namespace: "default"},
		Spec:       nestv1.DataResourceSpec{Type: "warehouse/trino", Tenant: "tenant-trinodelsvcer"},
	}

	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoServiceName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}

	injectedErr := fmt.Errorf("injected trino svc delete error")
	r, ctx := reconcilerWithDeleteError(t, injectedErr, trinoServiceName(dr), dr, existingSvc)
	if err := r.reconcileTrinoDelete(ctx, dr); err == nil {
		t.Fatal("reconcileTrinoDelete(svc delete error) expected error, got nil")
	}
}

func TestTrino_Delete_CoordCMDeleteError(t *testing.T) {
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "trino-del-ccm-err", Namespace: "default"},
		Spec:       nestv1.DataResourceSpec{Type: "warehouse/trino", Tenant: "tenant-trinodel-ccmerr"},
	}

	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoCoordinatorConfigMapName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}

	injectedErr := fmt.Errorf("injected trino coord CM delete error")
	r, ctx := reconcilerWithDeleteError(t, injectedErr, trinoCoordinatorConfigMapName(dr), dr, existingCM)
	if err := r.reconcileTrinoDelete(ctx, dr); err == nil {
		t.Fatal("reconcileTrinoDelete(coord CM delete error) expected error, got nil")
	}
}

func TestTrino_Delete_WorkerCMDeleteError(t *testing.T) {
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "trino-del-wcm-err", Namespace: "default"},
		Spec:       nestv1.DataResourceSpec{Type: "warehouse/trino", Tenant: "tenant-trinodelwcmerr"},
	}

	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trinoWorkerConfigMapName(dr),
			Namespace: dr.Spec.Tenant,
		},
	}

	injectedErr := fmt.Errorf("injected trino worker CM delete error")
	r, ctx := reconcilerWithDeleteError(t, injectedErr, trinoWorkerConfigMapName(dr), dr, existingCM)
	if err := r.reconcileTrinoDelete(ctx, dr); err == nil {
		t.Fatal("reconcileTrinoDelete(worker CM delete error) expected error, got nil")
	}
}

func TestDarkDrive_ForeignFS_EraseConfirmed_Adopted(t *testing.T) {
	dd := &nestv1.DarkDrive{
		ObjectMeta: metav1.ObjectMeta{Name: "nvme0n10", Namespace: "default"},
		Spec: nestv1.DarkDriveSpec{
			Node:           "node1",
			Class:          "nvme-hot",
			Signature:      "foreign-fs:ext4",
			EraseConfirmed: true, // confirmed → should proceed to adoption
			HardwarePool:   "pool-nvme",
		},
		Status: nestv1.DarkDriveStatus{
			State: nestv1.DarkDriveApproved,
		},
	}
	r, ctx := newDarkDriveReconciler(t, dd)
	if _, err := r.Reconcile(ctx, ddReq(dd)); err != nil {
		t.Fatalf("Reconcile(foreign-fs erase confirmed) error = %v", err)
	}
}
