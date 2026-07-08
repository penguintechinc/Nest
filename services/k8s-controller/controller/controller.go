package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/penguintechinc/nest/services/k8s-controller/pkg/config"
	"github.com/penguintechinc/nest/services/k8s-controller/pkg/models"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Controller manages the reconciliation loop for NEST resources
type Controller struct {
	config     *config.Config
	db         *gorm.DB
	clientset  *kubernetes.Clientset
	reconciler *Reconciler
	watcher    *Watcher
	log        *logrus.Entry
	stopChan   chan struct{}
	wg         sync.WaitGroup
	retryQueue map[uint]*retryEntry
	retryMutex sync.RWMutex
}

type retryEntry struct {
	resourceID uint
	retryCount int
	nextRetry  time.Time
}

// NewController creates a new controller instance
func NewController(cfg *config.Config, db *gorm.DB) (*Controller, error) {
	// Create Kubernetes clientset
	clientset, err := createK8sClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %w", err)
	}

	reconciler := NewReconciler(db, clientset)
	watcher := NewWatcher(clientset, cfg.NamespacePrefix)

	return &Controller{
		config:     cfg,
		db:         db,
		clientset:  clientset,
		reconciler: reconciler,
		watcher:    watcher,
		log:        logrus.WithField("component", "controller"),
		stopChan:   make(chan struct{}),
		retryQueue: make(map[uint]*retryEntry),
	}, nil
}

// Start begins the controller's reconciliation loop
func (c *Controller) Start(ctx context.Context) error {
	c.log.Info("Starting NEST Kubernetes controller")

	// Start event watcher
	if err := c.watcher.Start(ctx); err != nil {
		return fmt.Errorf("failed to start watcher: %w", err)
	}

	// Start worker goroutines
	for i := 0; i < c.config.WorkerCount; i++ {
		c.wg.Add(1)
		go c.reconcileWorker(ctx, i)
	}

	// Start event handler
	c.wg.Add(1)
	go c.eventHandler(ctx)

	// Start main reconciliation loop
	c.wg.Add(1)
	go c.reconcileLoop(ctx)

	c.log.WithField("workers", c.config.WorkerCount).Info("Controller started")

	return nil
}

// Stop gracefully stops the controller
func (c *Controller) Stop() {
	c.log.Info("Stopping controller")
	close(c.stopChan)
	c.wg.Wait()
	c.log.Info("Controller stopped")
}

// reconcileLoop is the main reconciliation loop
func (c *Controller) reconcileLoop(ctx context.Context) {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.ReconcileInterval)
	defer ticker.Stop()

	c.log.WithField("interval", c.config.ReconcileInterval).Info("Starting reconciliation loop")

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.reconcileAll(ctx)
		}
	}
}

// reconcileAll reconciles all resources with full lifecycle management
func (c *Controller) reconcileAll(ctx context.Context) {
	log := c.log.WithField("action", "reconcile_all")
	log.Debug("Starting full reconciliation")

	var resources []models.Resource
	if err := c.db.Where("lifecycle_mode = ? AND deleted_at IS NULL", "full").Find(&resources).Error; err != nil {
		log.WithError(err).Error("Failed to query resources")
		return
	}

	log.WithField("count", len(resources)).Info("Reconciling resources")

	for _, resource := range resources {
		// Check if resource is in retry queue
		if c.shouldSkipRetry(resource.ID) {
			continue
		}

		if err := c.reconciler.ReconcileResource(ctx, &resource); err != nil {
			log.WithFields(logrus.Fields{
				"resource_id": resource.ID,
				"error":       err,
			}).Error("Failed to reconcile resource")

			c.addToRetryQueue(resource.ID)
		} else {
			c.removeFromRetryQueue(resource.ID)
		}
	}

	log.Debug("Completed full reconciliation")
}

// eventHandler handles Kubernetes events from the watcher
func (c *Controller) eventHandler(ctx context.Context) {
	defer c.wg.Done()

	log := c.log.WithField("component", "event_handler")
	log.Info("Starting event handler")

	eventChan := c.watcher.GetEventChannel()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopChan:
			return
		case event := <-eventChan:
			c.handleEvent(ctx, event)
		}
	}
}

// handleEvent processes a single Kubernetes event
func (c *Controller) handleEvent(ctx context.Context, event ResourceEvent) {
	log := c.log.WithFields(logrus.Fields{
		"type":      event.Type,
		"namespace": event.Namespace,
		"name":      event.Name,
	})

	switch event.Type {
	case watch.Added, watch.Modified:
		switch res := event.Resource.(type) {
		case *appsv1.StatefulSet:
			c.handleStatefulSetEvent(ctx, res, log)
		case *corev1.Pod:
			c.handlePodEvent(ctx, res, log)
		}
	case watch.Deleted:
		log.Info("Resource deleted in Kubernetes")
	}
}

// handleStatefulSetEvent processes StatefulSet events
func (c *Controller) handleStatefulSetEvent(ctx context.Context, sts *appsv1.StatefulSet, log *logrus.Entry) {
	// Find resource by k8s name
	var resource models.Resource
	if err := c.db.Where("k8s_namespace = ? AND k8s_resource_name = ?",
		sts.Namespace, sts.Name).First(&resource).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			log.WithError(err).Error("Failed to query resource")
		}
		return
	}

	log = log.WithField("resource_id", resource.ID)

	// Update resource status based on StatefulSet status
	status := "active"
	if sts.Status.ReadyReplicas < sts.Status.Replicas {
		status = "updating"
	}

	connectionInfo := models.JSONMap{
		"ready_replicas": sts.Status.ReadyReplicas,
		"replicas":       sts.Status.Replicas,
		"service_name":   fmt.Sprintf("%s.%s.svc.cluster.local", sts.Name, sts.Namespace),
	}

	updates := map[string]interface{}{
		"status":          status,
		"connection_info": connectionInfo,
	}

	if err := c.db.Model(&models.Resource{}).Where("id = ?", resource.ID).Updates(updates).Error; err != nil {
		log.WithError(err).Error("Failed to update resource")
		return
	}

	log.WithField("status", status).Debug("Updated resource from StatefulSet event")
}

// handlePodEvent processes Pod events
func (c *Controller) handlePodEvent(ctx context.Context, pod *corev1.Pod, log *logrus.Entry) {
	// Get resource ID from pod labels
	resourceIDStr, ok := pod.Labels["resource-id"]
	if !ok {
		return
	}

	var resourceID uint
	if _, err := fmt.Sscanf(resourceIDStr, "%d", &resourceID); err != nil {
		return
	}

	log = log.WithFields(logrus.Fields{
		"resource_id": resourceID,
		"phase":       pod.Status.Phase,
	})

	// Check for pod failures
	if pod.Status.Phase == corev1.PodFailed {
		log.Warn("Pod failed, marking resource as degraded")

		updates := map[string]interface{}{
			"status": "error",
			"connection_info": models.JSONMap{
				"error": "Pod failed",
				"pod":   pod.Name,
			},
		}

		if err := c.db.Model(&models.Resource{}).Where("id = ?", resourceID).Updates(updates).Error; err != nil {
			log.WithError(err).Error("Failed to update resource")
		}
	}
}

// reconcileWorker is a worker goroutine for processing reconciliation tasks
func (c *Controller) reconcileWorker(ctx context.Context, id int) {
	defer c.wg.Done()

	log := c.log.WithField("worker_id", id)
	log.Info("Worker started")

	for {
		select {
		case <-ctx.Done():
			log.Info("Worker stopping")
			return
		case <-c.stopChan:
			log.Info("Worker stopping")
			return
		case <-time.After(1 * time.Second):
			// Workers can be extended to process from a work queue
			// For now, they handle event-driven reconciliation
		}
	}
}

// Retry queue management

func (c *Controller) shouldSkipRetry(resourceID uint) bool {
	c.retryMutex.RLock()
	defer c.retryMutex.RUnlock()

	entry, exists := c.retryQueue[resourceID]
	if !exists {
		return false
	}

	return time.Now().Before(entry.nextRetry)
}

func (c *Controller) addToRetryQueue(resourceID uint) {
	c.retryMutex.Lock()
	defer c.retryMutex.Unlock()

	entry, exists := c.retryQueue[resourceID]
	if !exists {
		entry = &retryEntry{
			resourceID: resourceID,
			retryCount: 0,
		}
		c.retryQueue[resourceID] = entry
	}

	entry.retryCount++
	backoff := c.calculateBackoff(entry.retryCount)
	entry.nextRetry = time.Now().Add(backoff)

	c.log.WithFields(logrus.Fields{
		"resource_id": resourceID,
		"retry_count": entry.retryCount,
		"next_retry":  entry.nextRetry,
	}).Warn("Added resource to retry queue")
}

func (c *Controller) removeFromRetryQueue(resourceID uint) {
	c.retryMutex.Lock()
	defer c.retryMutex.Unlock()

	if _, exists := c.retryQueue[resourceID]; exists {
		delete(c.retryQueue, resourceID)
		c.log.WithField("resource_id", resourceID).Debug("Removed resource from retry queue")
	}
}

func (c *Controller) calculateBackoff(retryCount int) time.Duration {
	backoff := c.config.BackoffBase * time.Duration(1<<uint(retryCount-1))
	if backoff > c.config.BackoffMax {
		backoff = c.config.BackoffMax
	}
	return backoff
}

// createK8sClient creates a Kubernetes client
func createK8sClient(cfg *config.Config) (*kubernetes.Clientset, error) {
	var k8sConfig *rest.Config
	var err error

	if cfg.InCluster {
		k8sConfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
		}
	} else {
		k8sConfig, err = clientcmd.BuildConfigFromFlags("", cfg.KubeConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to build kubeconfig: %w", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return clientset, nil
}
