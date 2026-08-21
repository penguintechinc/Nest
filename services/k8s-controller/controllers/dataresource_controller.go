package controllers

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// DataResourceReconciler reconciles DataResource CRDs.
// It translates DataResource specs into upstream operator CRs
// (e.g., CloudNativePG Cluster, Strimzi Kafka, etc.).
type DataResourceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=nest.penguintech.io,resources=dataresources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nest.penguintech.io,resources=dataresources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nest.penguintech.io,resources=dataresources/finalizers,verbs=update

func (r *DataResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var dr nestv1.DataResource
	if err := r.Get(ctx, req.NamespacedName, &dr); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("reconciling DataResource",
		"name", dr.Name,
		"tenant", dr.Spec.Tenant,
		"type", dr.Spec.Type,
		"phase", dr.Status.Phase,
	)

	// Add finalizer
	if !containsString(dr.Finalizers, "nest.penguintech.io/dataresource") {
		dr.Finalizers = append(dr.Finalizers, "nest.penguintech.io/dataresource")
		if err := r.Update(ctx, &dr); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Handle deletion
	if !dr.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &dr)
	}

	return r.reconcileCreate(ctx, &dr)
}

func (r *DataResourceReconciler) reconcileCreate(ctx context.Context, dr *nestv1.DataResource) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Check tenant quota before every provisioning attempt (not just initial phase)
	var tenant nestv1.Tenant
	if err := r.Get(ctx, client.ObjectKey{Name: dr.Spec.Tenant, Namespace: dr.Namespace}, &tenant); err == nil {
		for _, cond := range tenant.Status.Conditions {
			if cond.Type == "QuotaExceeded" && cond.Status == metav1.ConditionTrue {
				// Quota exceeded - mark DataResource as Failed and return requeue
				r.setPhase(dr, nestv1.PhaseFailed, cond.Message)
				_ = r.Status().Update(ctx, dr)
				logger.Info("DataResource provisioning blocked by quota", "tenant", dr.Spec.Tenant, "message", cond.Message)
				return ctrl.Result{}, nil
			}
		}
	} else if !errors.IsNotFound(err) {
		logger.Error(err, "failed to check tenant quota")
		return ctrl.Result{}, err
	}

	// If already Ready, reconcile idempotently to detect spec changes (observedGeneration check)
	// Only skip reconciliation if no spec updates have occurred
	if dr.Status.Phase == nestv1.PhaseReady {
		// Check if spec has changed since last reconciliation
		if dr.Status.ObservedGeneration >= dr.Generation {
			return ctrl.Result{}, nil
		}
		// Spec changed, continue to update children
	}

	if dr.Status.Phase == "" || dr.Status.Phase == nestv1.PhasePending {
		r.setPhase(dr, nestv1.PhaseProvisioning, "Provisioning started")
		if err := r.Status().Update(ctx, dr); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Handle external resources first
	if dr.Spec.Origination == nestv1.OriginationExternal {
		if err := r.reconcileExternal(ctx, dr); err != nil {
			logger.Error(err, "external provisioning failed")
			r.setPhase(dr, nestv1.PhaseFailed, err.Error())
			_ = r.Status().Update(ctx, dr)
			return ctrl.Result{}, err
		}
		// For external resources, re-queue to wait for provider readiness
		if dr.Status.Phase == nestv1.PhaseProvisioning {
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		return ctrl.Result{}, nil
	}

	// Handle imported (adopted, pre-existing) resources before the provisioning
	// dispatch below. Without this branch an imported resource falls through to
	// the engine switch and provisions a NEW in-cluster cluster — the exact
	// opposite of adopting the caller's existing one.
	if dr.Spec.Origination == nestv1.OriginationImported {
		if err := r.reconcileImported(ctx, dr); err != nil {
			logger.Error(err, "imported adoption failed")
			r.setPhase(dr, nestv1.PhaseFailed, err.Error())
			_ = r.Status().Update(ctx, dr)
			return ctrl.Result{}, err
		}
		// Re-probe periodically: an adopted resource's health changes without
		// any spec change to trigger a watch event.
		return ctrl.Result{RequeueAfter: importedHealthInterval}, nil
	}

	// Dispatch to engine-specific provisioner
	var err error
	switch dr.Spec.Type {
	case "postgres":
		err = r.reconcilePostgres(ctx, dr)
	case "mariadb":
		err = r.reconcileMariaDB(ctx, dr)
	case "mysql":
		err = r.reconcileMySQL(ctx, dr)
	case "object":
		err = r.reconcileObject(ctx, dr)
	case "pvc/block":
		err = r.reconcilePVCBlock(ctx, dr)
	case "pvc/file":
		err = r.reconcilePVCFile(ctx, dr)
	case "keyvalue":
		err = r.reconcileKeyvalue(ctx, dr)
	case "kafka":
		err = r.reconcileKafka(ctx, dr)
	case "search":
		err = r.reconcileOpenSearch(ctx, dr)
	case "rockfs":
		err = r.reconcileFerretDB(ctx, dr)
	case "vector":
		err = r.reconcileVector(ctx, dr)
	case "clickhouse":
		err = r.reconcileClickhouse(ctx, dr)
	case "timeseries":
		err = r.reconcileTimeseries(ctx, dr)
	case "warehouse/trino":
		err = r.reconcileTrino(ctx, dr)
	case "lakehouse/iceberg":
		err = r.reconcileIceberg(ctx, dr)
	case "nfs":
		err = r.reconcileNFS(ctx, dr)
	case "iscsi":
		err = r.reconcileISCSI(ctx, dr)
	case "filesystem":
		err = r.reconcileFilesystem(ctx, dr)
	default:
		err = fmt.Errorf("unsupported DataResource type: %s", dr.Spec.Type)
	}

	if err != nil {
		logger.Error(err, "provisioning failed")
		r.setPhase(dr, nestv1.PhaseFailed, err.Error())
		_ = r.Status().Update(ctx, dr)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *DataResourceReconciler) reconcileDelete(ctx context.Context, dr *nestv1.DataResource) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("deleting DataResource", "name", dr.Name)

	// Handle external resources first
	if dr.Spec.Origination == nestv1.OriginationExternal {
		if err := r.reconcileExternalDelete(ctx, dr); err != nil {
			logger.Error(err, "failed to delete external resource", "name", dr.Name)
			return ctrl.Result{}, err
		}
		// Only remove finalizer after successful cleanup
		dr.Finalizers = removeString(dr.Finalizers, "nest.penguintech.io/dataresource")
		return ctrl.Result{}, r.Update(ctx, dr)
	}

	// Imported resources are adopted, not owned. Deleting the DataResource must
	// release Nest's reference and nothing more — the caller's database keeps
	// running. Falling through to the engine delete handlers below would be
	// only accidentally safe (the in-cluster object happens not to exist), so
	// return explicitly rather than relying on that.
	if dr.Spec.Origination == nestv1.OriginationImported {
		logger.Info("releasing imported DataResource; external resource is left untouched",
			"name", dr.Name, "type", dr.Spec.Type)
		dr.Finalizers = removeString(dr.Finalizers, "nest.penguintech.io/dataresource")
		return ctrl.Result{}, r.Update(ctx, dr)
	}

	// Dispatch to engine-specific delete handler, collecting errors
	var deleteErr error
	switch dr.Spec.Type {
	case "postgres":
		deleteErr = r.reconcilePostgresDelete(ctx, dr)
	case "mariadb":
		deleteErr = r.reconcileMariaDBDelete(ctx, dr)
	case "mysql":
		deleteErr = r.reconcileMySQLDelete(ctx, dr)
	case "object":
		deleteErr = r.reconcileObjectDelete(ctx, dr)
	case "pvc/block":
		deleteErr = r.reconcilePVCBlockDelete(ctx, dr)
	case "pvc/file":
		deleteErr = r.reconcilePVCFileDelete(ctx, dr)
	case "keyvalue":
		deleteErr = r.reconcileKeyvalueDelete(ctx, dr)
	case "kafka":
		deleteErr = r.reconcileKafkaDelete(ctx, dr)
	case "search":
		deleteErr = r.reconcileOpenSearchDelete(ctx, dr)
	case "rockfs":
		deleteErr = r.reconcileFerretDBDelete(ctx, dr)
	case "vector":
		deleteErr = r.reconcileVectorDelete(ctx, dr)
	case "clickhouse":
		deleteErr = r.reconcileClickhouseDelete(ctx, dr)
	case "timeseries":
		deleteErr = r.reconcileTimeseriesDelete(ctx, dr)
	case "warehouse/trino":
		deleteErr = r.reconcileTrinoDelete(ctx, dr)
	case "lakehouse/iceberg":
		deleteErr = r.reconcileIcebergDelete(ctx, dr)
	case "nfs":
		deleteErr = r.reconcileNFSDelete(ctx, dr)
	case "iscsi":
		deleteErr = r.reconcileISCSIDelete(ctx, dr)
	case "filesystem":
		deleteErr = r.reconcileFilesystemDelete(ctx, dr)
	}

	// Return error if delete failed; only remove finalizer after successful cleanup
	if deleteErr != nil {
		logger.Error(deleteErr, "delete operation failed, will retry", "name", dr.Name)
		return ctrl.Result{}, deleteErr
	}

	dr.Finalizers = removeString(dr.Finalizers, "nest.penguintech.io/dataresource")
	return ctrl.Result{}, r.Update(ctx, dr)
}

func (r *DataResourceReconciler) setPhase(dr *nestv1.DataResource, phase nestv1.DataResourcePhase, msg string) {
	dr.Status.Phase = phase
	meta.SetStatusCondition(&dr.Status.Conditions, metav1.Condition{
		Type:               string(phase),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: dr.Generation,
		Reason:             string(phase),
		Message:            msg,
	})
}

func (r *DataResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nestv1.DataResource{}).
		// Watch for changes to child resources so spec updates trigger reconciliation
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&appsv1.StatefulSet{}).
		// CloudNativePG Cluster and other external CRs via owner references
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) []string {
	result := make([]string, 0, len(slice))
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}

// ensureTenantNamespace creates a tenant namespace with security defaults (network policies, labels).
// The namespace name is the tenant ID, so it carries nest.penguintech.io/tenant=<ns>; cross-namespace
// NetworkPolicies and other controllers select tenant namespaces by that label rather than by name.
func (r *DataResourceReconciler) ensureTenantNamespace(ctx context.Context, ns string) error {
	logger := log.FromContext(ctx)

	wantLabels := map[string]string{
		"nest.penguintech.io/managed": "true",
		"nest.penguintech.io/tenant":  ns,
	}

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   ns,
			Labels: wantLabels,
		},
	}
	if err := r.Create(ctx, namespace); err != nil {
		if !errors.IsAlreadyExists(err) {
			logger.Error(err, "failed to create namespace", "namespace", ns)
			return fmt.Errorf("creating namespace %s: %w", ns, err)
		}
		// Namespace predates the tenant label (or was created out-of-band); backfill
		// the labels so policy selectors still match it.
		if err := r.ensureNamespaceLabels(ctx, ns, wantLabels); err != nil {
			logger.Error(err, "failed to backfill tenant namespace labels", "namespace", ns)
			return err
		}
	}

	// Ensure default-deny NetworkPolicy for security (prevent cross-tenant lateral movement)
	if err := r.ensureTenantNetworkPolicies(ctx, ns); err != nil {
		logger.Error(err, "failed to ensure tenant network policies", "namespace", ns)
		// Continue even if NetworkPolicy creation fails; it's a security hardening step
	}

	return nil
}

// ensureNamespaceLabels backfills the given labels onto an existing namespace,
// leaving any operator-set labels untouched. It is a no-op when they already match.
func (r *DataResourceReconciler) ensureNamespaceLabels(ctx context.Context, ns string, want map[string]string) error {
	existing := &corev1.Namespace{}
	if err := r.Get(ctx, client.ObjectKey{Name: ns}, existing); err != nil {
		return fmt.Errorf("getting namespace %s: %w", ns, err)
	}

	patch := client.MergeFrom(existing.DeepCopy())
	changed := false
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	for k, v := range want {
		if existing.Labels[k] != v {
			existing.Labels[k] = v
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if err := r.Patch(ctx, existing, patch); err != nil {
		return fmt.Errorf("patching namespace %s labels: %w", ns, err)
	}
	return nil
}

// ensureTenantNetworkPolicies ensures a default-deny NetworkPolicy is in place for the tenant namespace.
// Allows:
// - Same-namespace pod-to-pod traffic (tenant's own workloads)
// - DNS egress to kube-system (UDP/TCP 53)
// - Egress to controller/gateway services (narrowly scoped for provisioning)
func (r *DataResourceReconciler) ensureTenantNetworkPolicies(ctx context.Context, ns string) error {
	logger := log.FromContext(ctx)

	policyName := "nest-default-deny"

	// Check if policy already exists (idempotent)
	existingPolicy := &networking.NetworkPolicy{}
	err := r.Get(ctx, client.ObjectKey{Name: policyName, Namespace: ns}, existingPolicy)
	if err == nil {
		// Policy already exists, skip creation (idempotent)
		return nil
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to check NetworkPolicy: %w", err)
	}

	// Create default-deny NetworkPolicy with allow rules
	policy := &networking.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: ns,
			Labels: map[string]string{
				"nest.penguintech.io/managed": "true",
				"nest.penguintech.io/type":    "default-deny",
			},
		},
		Spec: networking.NetworkPolicySpec{
			// Default deny all ingress and egress
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networking.PolicyType{
				networking.PolicyTypeIngress,
				networking.PolicyTypeEgress,
			},
			// Allow ingress: same-namespace pod-to-pod
			Ingress: []networking.NetworkPolicyIngressRule{
				{
					From: []networking.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{},
						},
					},
				},
			},
			// Allow egress: same-namespace, DNS, and controller/gateway endpoints
			Egress: []networking.NetworkPolicyEgressRule{
				// Allow to same namespace
				{
					To: []networking.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{},
						},
					},
				},
				// Allow DNS to kube-system
				{
					To: []networking.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": "kube-system",
								},
							},
						},
					},
					Ports: []networking.NetworkPolicyPort{
						{
							Protocol: &udpProto,
							Port:     &dnsPort,
						},
						{
							Protocol: &tcpProto,
							Port:     &dnsPort,
						},
					},
				},
				// Allow to controller namespace (for gateway/provisioning)
				// Restrict to known gateway services by app.kubernetes.io/name label
				{
					To: []networking.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": "nest-controller",
								},
							},
							PodSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "app.kubernetes.io/name",
										Operator: metav1.LabelSelectorOpIn,
										Values: []string{
											"nest-gateway",
											"nest-iscsi-gateway",
											"nest-nfs-gateway",
										},
									},
								},
							},
						},
					},
					Ports: []networking.NetworkPolicyPort{
						{
							Protocol: &tcpProto,
							Port:     &http8080Port,
						},
						{
							Protocol: &tcpProto,
							Port:     &http8082Port,
						},
						{
							Protocol: &tcpProto,
							Port:     &http8083Port,
						},
					},
				},
			},
		},
	}

	if err := r.Create(ctx, policy); err != nil {
		logger.Error(err, "failed to create default-deny NetworkPolicy", "namespace", ns)
		return fmt.Errorf("creating NetworkPolicy: %w", err)
	}

	logger.Info("created default-deny NetworkPolicy for tenant namespace", "namespace", ns)
	return nil
}

// Port definitions for network policies
var (
	dnsPort      = intstr.FromInt(53)
	http8080Port = intstr.FromInt(8080)
	http8082Port = intstr.FromInt(8082)
	http8083Port = intstr.FromInt(8083)
	tcpProto     = corev1.ProtocolTCP
	udpProto     = corev1.ProtocolUDP
)
