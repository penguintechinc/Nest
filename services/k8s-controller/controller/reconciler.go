package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/penguintechinc/nest/services/k8s-controller/pkg/models"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"gorm.io/gorm"
)

// Reconciler handles reconciliation of resources
type Reconciler struct {
	db        *gorm.DB
	clientset *kubernetes.Clientset
	log       *logrus.Entry
}

// NewReconciler creates a new reconciler instance
func NewReconciler(db *gorm.DB, clientset *kubernetes.Clientset) *Reconciler {
	return &Reconciler{
		db:        db,
		clientset: clientset,
		log:       logrus.WithField("component", "reconciler"),
	}
}

// ReconcileResource reconciles a single resource
func (r *Reconciler) ReconcileResource(ctx context.Context, resource *models.Resource) error {
	log := r.log.WithFields(logrus.Fields{
		"resource_id":   resource.ID,
		"resource_name": resource.Name,
		"lifecycle":     resource.LifecycleMode,
		"status":        resource.Status,
	})

	log.Debug("Reconciling resource")

	// Only reconcile resources with full lifecycle management
	if resource.LifecycleMode != "full" {
		log.Debug("Skipping non-full lifecycle resource")
		return nil
	}

	// Handle deleted resources
	if resource.DeletedAt != nil {
		return r.reconcileDelete(ctx, resource, log)
	}

	// Get resource type to determine how to provision
	var resourceType models.ResourceType
	if err := r.db.First(&resourceType, resource.ResourceTypeID).Error; err != nil {
		return fmt.Errorf("failed to get resource type: %w", err)
	}

	// Check if resource exists in Kubernetes
	exists, currentState, err := r.getK8sState(ctx, resource)
	if err != nil {
		return fmt.Errorf("failed to get k8s state: %w", err)
	}

	if !exists {
		// Create resource in Kubernetes
		return r.reconcileCreate(ctx, resource, resourceType, log)
	}

	// Reconcile existing resource
	return r.reconcileUpdate(ctx, resource, resourceType, currentState, log)
}

// reconcileCreate creates a new resource in Kubernetes
func (r *Reconciler) reconcileCreate(ctx context.Context, resource *models.Resource,
	resourceType models.ResourceType, log *logrus.Entry) error {
	log.Info("Creating resource in Kubernetes")

	// Update status to provisioning
	if err := r.updateResourceStatus(resource.ID, "provisioning", nil); err != nil {
		return err
	}

	// Create provisioning job
	job := &models.ProvisioningJob{
		ResourceID: resource.ID,
		JobType:    "create",
		Status:     "running",
		StartedAt:  timePtr(time.Now()),
	}
	if err := r.db.Create(job).Error; err != nil {
		log.WithError(err).Error("Failed to create provisioning job")
	}

	// Create StatefulSet based on resource type
	sts, err := r.buildStatefulSet(resource, resourceType)
	if err != nil {
		r.failJob(job.ID, fmt.Sprintf("Failed to build StatefulSet: %v", err))
		return r.updateResourceStatus(resource.ID, "error", map[string]interface{}{
			"error": err.Error(),
		})
	}

	// Ensure namespace exists
	if err := r.ensureNamespace(ctx, *resource.K8sNamespace); err != nil {
		r.failJob(job.ID, fmt.Sprintf("Failed to ensure namespace: %v", err))
		return fmt.Errorf("failed to ensure namespace: %w", err)
	}

	// Create the StatefulSet
	created, err := r.clientset.AppsV1().StatefulSets(*resource.K8sNamespace).Create(
		ctx, sts, metav1.CreateOptions{})
	if err != nil {
		r.failJob(job.ID, fmt.Sprintf("Failed to create StatefulSet: %v", err))
		return r.updateResourceStatus(resource.ID, "error", map[string]interface{}{
			"error": err.Error(),
		})
	}

	log.WithField("statefulset", created.Name).Info("StatefulSet created")

	// Update resource with k8s information
	updates := map[string]interface{}{
		"k8s_namespace":      created.Namespace,
		"k8s_resource_name":  created.Name,
		"k8s_resource_type":  "StatefulSet",
		"status":             "active",
	}

	if err := r.db.Model(&models.Resource{}).Where("id = ?", resource.ID).Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to update resource: %w", err)
	}

	// Complete job
	r.completeJob(job.ID, "Resource created successfully")

	// Create audit log
	r.createAuditLog("resource.created", "resources", resource.ID, resource.TeamID, nil)

	return nil
}

// reconcileUpdate updates an existing resource in Kubernetes
func (r *Reconciler) reconcileUpdate(ctx context.Context, resource *models.Resource,
	resourceType models.ResourceType, currentState *appsv1.StatefulSet, log *logrus.Entry) error {

	log.Debug("Updating resource in Kubernetes")

	// Check if update is needed
	desiredState, err := r.buildStatefulSet(resource, resourceType)
	if err != nil {
		return fmt.Errorf("failed to build desired state: %w", err)
	}

	needsUpdate := false

	// Check replicas
	if desiredState.Spec.Replicas != nil && currentState.Spec.Replicas != nil {
		if *desiredState.Spec.Replicas != *currentState.Spec.Replicas {
			needsUpdate = true
			log.WithFields(logrus.Fields{
				"current": *currentState.Spec.Replicas,
				"desired": *desiredState.Spec.Replicas,
			}).Info("Replica count mismatch")
		}
	}

	if needsUpdate {
		// Update the StatefulSet
		currentState.Spec.Replicas = desiredState.Spec.Replicas
		_, err := r.clientset.AppsV1().StatefulSets(*resource.K8sNamespace).Update(
			ctx, currentState, metav1.UpdateOptions{})
		if err != nil {
			return r.updateResourceStatus(resource.ID, "error", map[string]interface{}{
				"error": err.Error(),
			})
		}

		log.Info("StatefulSet updated")
		r.createAuditLog("resource.updated", "resources", resource.ID, resource.TeamID, nil)
	}

	// Update connection info from StatefulSet status
	return r.updateConnectionInfo(ctx, resource, currentState)
}

// reconcileDelete deletes a resource from Kubernetes
func (r *Reconciler) reconcileDelete(ctx context.Context, resource *models.Resource, log *logrus.Entry) error {
	log.Info("Deleting resource from Kubernetes")

	if resource.K8sNamespace == nil || resource.K8sResourceName == nil {
		log.Warn("Resource has no k8s information, marking as deleted")
		return r.updateResourceStatus(resource.ID, "deleted", nil)
	}

	// Delete the StatefulSet
	err := r.clientset.AppsV1().StatefulSets(*resource.K8sNamespace).Delete(
		ctx, *resource.K8sResourceName, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete StatefulSet: %w", err)
	}

	log.Info("StatefulSet deleted")

	// Update resource status
	if err := r.updateResourceStatus(resource.ID, "deleted", nil); err != nil {
		return err
	}

	r.createAuditLog("resource.deleted", "resources", resource.ID, resource.TeamID, nil)

	return nil
}

// getK8sState gets the current state of a resource in Kubernetes
func (r *Reconciler) getK8sState(ctx context.Context, resource *models.Resource) (bool, *appsv1.StatefulSet, error) {
	if resource.K8sNamespace == nil || resource.K8sResourceName == nil {
		return false, nil, nil
	}

	sts, err := r.clientset.AppsV1().StatefulSets(*resource.K8sNamespace).Get(
		ctx, *resource.K8sResourceName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}

	return true, sts, nil
}

// buildStatefulSet creates a StatefulSet spec from a resource
func (r *Reconciler) buildStatefulSet(resource *models.Resource, resourceType models.ResourceType) (*appsv1.StatefulSet, error) {
	// Extract replicas from config
	replicas := int32(1)
	if resource.Config != nil {
		if replicasVal, ok := resource.Config["replicas"].(float64); ok {
			replicas = int32(replicasVal)
		}
	}

	// Build StatefulSet based on resource type
	image := ""
	port := int32(5432)

	switch resourceType.Name {
	case "postgresql":
		image = "postgres:16-alpine"
		port = 5432
	case "mariadb":
		image = "mariadb:11-jammy"
		port = 3306
	case "redis":
		image = "redis:7-alpine"
		port = 6379
	default:
		return nil, fmt.Errorf("unsupported resource type: %s", resourceType.Name)
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resource.Name,
			Namespace: *resource.K8sNamespace,
			Labels: map[string]string{
				"app":         resource.Name,
				"managed-by":  "nest-controller",
				"resource-id": fmt.Sprintf("%d", resource.ID),
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": resource.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":         resource.Name,
						"managed-by":  "nest-controller",
						"resource-id": fmt.Sprintf("%d", resource.ID),
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  resourceType.Name,
							Image: image,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: port,
									Name:          resourceType.Name,
								},
							},
						},
					},
				},
			},
		},
	}

	return sts, nil
}

// updateConnectionInfo updates resource connection info from k8s state
func (r *Reconciler) updateConnectionInfo(ctx context.Context, resource *models.Resource,
	sts *appsv1.StatefulSet) error {

	// Get pods for this StatefulSet
	pods, err := r.clientset.CoreV1().Pods(*resource.K8sNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", resource.Name),
	})
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	// Extract pod IPs and status
	podIPs := []string{}
	allReady := true
	for _, pod := range pods.Items {
		if pod.Status.PodIP != "" {
			podIPs = append(podIPs, pod.Status.PodIP)
		}
		if pod.Status.Phase != corev1.PodRunning {
			allReady = false
		}
	}

	// Update connection info
	connectionInfo := models.JSONMap{
		"pod_ips":       podIPs,
		"ready_replicas": sts.Status.ReadyReplicas,
		"replicas":      sts.Status.Replicas,
		"service_name":  fmt.Sprintf("%s.%s.svc.cluster.local", resource.Name, *resource.K8sNamespace),
	}

	status := "active"
	if !allReady || sts.Status.ReadyReplicas < sts.Status.Replicas {
		status = "updating"
	}

	updates := map[string]interface{}{
		"connection_info": connectionInfo,
		"status":          status,
	}

	return r.db.Model(&models.Resource{}).Where("id = ?", resource.ID).Updates(updates).Error
}

// Helper functions

func (r *Reconciler) updateResourceStatus(id uint, status string, info map[string]interface{}) error {
	updates := map[string]interface{}{"status": status}
	if info != nil {
		updates["connection_info"] = info
	}
	return r.db.Model(&models.Resource{}).Where("id = ?", id).Updates(updates).Error
}

func (r *Reconciler) ensureNamespace(ctx context.Context, namespace string) error {
	_, err := r.clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err == nil {
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
			Labels: map[string]string{
				"managed-by": "nest-controller",
			},
		},
	}

	_, err = r.clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	return err
}

func (r *Reconciler) completeJob(id uint, message string) {
	now := time.Now()
	r.db.Model(&models.ProvisioningJob{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":       "completed",
		"completed_at": &now,
		"logs":         &message,
	})
}

func (r *Reconciler) failJob(id uint, message string) {
	now := time.Now()
	r.db.Model(&models.ProvisioningJob{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":        "failed",
		"completed_at":  &now,
		"error_message": &message,
	})
}

func (r *Reconciler) createAuditLog(action, resourceType string, resourceID, teamID uint, details map[string]interface{}) {
	detailsJSON := models.JSONMap(details)
	resType := resourceType
	resID := resourceID
	log := &models.AuditLog{
		Action:       action,
		ResourceType: &resType,
		ResourceID:   &resID,
		TeamID:       &teamID,
		Details:      detailsJSON,
	}
	r.db.Create(log)
}

func timePtr(t time.Time) *time.Time {
	return &t
}
