//go:build integration
// +build integration

package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

const (
	namespace    = "nest"
	kubeContext  = "docker-desktop"
	pollTimeout  = 120 * time.Second
	pollInterval = 5 * time.Second
)

// getKubeConfig returns the Kubernetes config for the docker-desktop context.
// Respects KUBECONFIG and KUBE_CONTEXT env vars for override.
func getKubeConfig() (*rest.Config, error) {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = clientcmd.RecommendedHomeFile
	}
	ctx := os.Getenv("KUBE_CONTEXT")
	if ctx == "" {
		ctx = kubeContext
	}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig},
		&clientcmd.ConfigOverrides{CurrentContext: ctx},
	).ClientConfig()
}

// getKubeClient returns a controller-runtime client.Client for the docker-desktop context.
func getKubeClient(t *testing.T) client.Client {
	config, err := getKubeConfig()
	if err != nil {
		t.Fatalf("failed to get kubeconfig: %v", err)
	}

	// Create scheme and register types
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add apiextensionsv1 to scheme: %v", err)
	}
	if err := nestv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add nestv1 to scheme: %v", err)
	}

	cl, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("failed to create kubernetes client: %v", err)
	}
	return cl
}

// isClusterAvailable checks if the cluster is available by attempting to get server info.
func isClusterAvailable(t *testing.T) bool {
	config, err := getKubeConfig()
	if err != nil {
		return false
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return false
	}

	_, err = clientset.ServerVersion()
	if err != nil {
		t.Logf("cluster unavailable: %v", err)
		return false
	}
	return true
}

// startPortForward starts a kubectl port-forward and returns a cleanup function.
func startPortForward(t *testing.T, resourceType, resourceName, namespace string, localPort, remotePort int) {
	cmd := exec.Command(
		"kubectl", "port-forward",
		fmt.Sprintf("%s/%s", resourceType, resourceName),
		fmt.Sprintf("%d:%d", localPort, remotePort),
		"--context", kubeContext,
		"-n", namespace,
	)

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start port-forward: %v", err)
	}

	// Give port-forward time to establish
	time.Sleep(1 * time.Second)

	t.Cleanup(func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	})
}

// getRESTClient returns a REST client for API extensions.
func getRESTClient(t *testing.T) rest.Interface {
	config, err := getKubeConfig()
	if err != nil {
		t.Fatalf("failed to get kubeconfig: %v", err)
	}

	return apiextensionsclient.NewForConfigOrDie(config).
		ApiextensionsV1().
		RESTClient()
}

// TestCRDsInstalled verifies that all required CRDs are installed.
func TestCRDsInstalled(t *testing.T) {
	if !isClusterAvailable(t) {
		t.Skip("cluster not available")
	}

	config, err := getKubeConfig()
	if err != nil {
		t.Fatalf("failed to get kubeconfig: %v", err)
	}

	clientset, err := apiextensionsclient.NewForConfig(config)
	if err != nil {
		t.Fatalf("failed to create apiextensions clientset: %v", err)
	}

	crdClient := clientset.ApiextensionsV1().CustomResourceDefinitions()

	requiredCRDs := []string{
		"dataresources.nest.penguintech.io",
		"tenants.nest.penguintech.io",
		"credentials.nest.penguintech.io",
		"dataprotectionpolicies.nest.penguintech.io",
	}

	crdCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, crdName := range requiredCRDs {
		_, err := crdClient.Get(crdCtx, crdName, metav1.GetOptions{})
		if err != nil {
			t.Errorf("CRD %s not found: %v", crdName, err)
		} else {
			t.Logf("CRD %s installed", crdName)
		}
	}
}

// TestPodsRunning verifies that required pods are running and ready.
func TestPodsRunning(t *testing.T) {
	if !isClusterAvailable(t) {
		t.Skip("cluster not available")
	}

	config, err := getKubeConfig()
	if err != nil {
		t.Fatalf("failed to get kubeconfig: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatalf("failed to create kubernetes clientset: %v", err)
	}

	podClient := clientset.CoreV1().Pods(namespace)

	requiredLabels := map[string]string{
		"app=nest-api":        "nest-api",
		"app=nest-controller": "nest-controller",
		"app=nest-gateway":    "nest-gateway",
		"app=nest-scheduler":  "nest-scheduler",
	}

	podCtx := context.Background()

	for labelSelector, appName := range requiredLabels {
		err := wait.PollUntilContextTimeout(podCtx, pollInterval, pollTimeout, true, func(pollCtx context.Context) (bool, error) {
			pods, err := podClient.List(pollCtx, metav1.ListOptions{LabelSelector: labelSelector})
			if err != nil {
				t.Logf("error listing pods with label %s: %v", labelSelector, err)
				return false, nil
			}

			if len(pods.Items) == 0 {
				t.Logf("no pods found for %s", appName)
				return false, nil
			}

			// Check if at least one pod is running and ready
			for _, pod := range pods.Items {
				if pod.Status.Phase == corev1.PodRunning {
					for _, cond := range pod.Status.Conditions {
						if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
							t.Logf("pod %s/%s is running and ready", pod.Namespace, pod.Name)
							return true, nil
						}
					}
				}
			}
			return false, nil
		})

		if err != nil {
			t.Errorf("pod with label %s not running and ready within timeout: %v", labelSelector, err)
		}
	}
}

// HealthCheckResponse represents a health check response.
type HealthCheckResponse struct {
	Status string `json:"status"`
}

// TestGatewayHealth verifies the gateway health endpoint.
func TestGatewayHealth(t *testing.T) {
	if !isClusterAvailable(t) {
		t.Skip("cluster not available")
	}

	localPort := 8082
	remotePort := 8082
	startPortForward(t, "svc", "nest-gateway", namespace, localPort, remotePort)

	gCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := wait.PollUntilContextTimeout(gCtx, 1*time.Second, 10*time.Second, true, func(pollCtx context.Context) (bool, error) {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", localPort))
		if err != nil {
			return false, nil
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Logf("gateway health check returned status %d", resp.StatusCode)
			return false, nil
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return false, nil
		}

		var healthResp HealthCheckResponse
		if err := json.Unmarshal(body, &healthResp); err != nil {
			t.Logf("failed to parse health response: %v", err)
			return false, nil
		}

		if healthResp.Status != "ok" {
			t.Logf("gateway health status is %s, expected ok", healthResp.Status)
			return false, nil
		}

		t.Log("gateway health check passed")
		return true, nil
	})

	if err != nil {
		t.Errorf("gateway health check failed: %v", err)
	}
}

// TestControllerHealth verifies the controller health endpoint.
func TestControllerHealth(t *testing.T) {
	if !isClusterAvailable(t) {
		t.Skip("cluster not available")
	}

	localPort := 8081
	remotePort := 8081
	startPortForward(t, "svc", "nest-controller", namespace, localPort, remotePort)

	cCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := wait.PollUntilContextTimeout(cCtx, 1*time.Second, 10*time.Second, true, func(pollCtx context.Context) (bool, error) {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/healthz", localPort))
		if err != nil {
			return false, nil
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Logf("controller health check returned status %d", resp.StatusCode)
			return false, nil
		}

		t.Log("controller health check passed")
		return true, nil
	})

	if err != nil {
		t.Errorf("controller health check failed: %v", err)
	}
}

// TestAPIHealth verifies the API health endpoint.
func TestAPIHealth(t *testing.T) {
	if !isClusterAvailable(t) {
		t.Skip("cluster not available")
	}

	localPort := 8080
	remotePort := 8080
	startPortForward(t, "svc", "nest-api", namespace, localPort, remotePort)

	aCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := wait.PollUntilContextTimeout(aCtx, 1*time.Second, 10*time.Second, true, func(pollCtx context.Context) (bool, error) {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", localPort))
		if err != nil {
			return false, nil
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Logf("API health check returned status %d", resp.StatusCode)
			return false, nil
		}

		t.Log("API health check passed")
		return true, nil
	})

	if err != nil {
		t.Errorf("API health check failed: %v", err)
	}
}

// TestDataResourceLifecycle verifies DataResource CRUD operations.
func TestDataResourceLifecycle(t *testing.T) {
	if !isClusterAvailable(t) {
		t.Skip("cluster not available")
	}

	cl := getKubeClient(t)
	drCtx := context.Background()

	// Create a DataResource
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-postgres-dr",
			Namespace: namespace,
			Labels: map[string]string{
				"nest.penguintech.io/tenant": "nest",
			},
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "postgres",
			Class:  "default",
			Tenant: "nest",
		},
	}

	if err := cl.Create(drCtx, dr); err != nil {
		t.Fatalf("failed to create DataResource: %v", err)
	}
	t.Log("DataResource created successfully")

	// Wait for the DataResource to appear
	createdDR := &nestv1.DataResource{}
	err := wait.PollUntilContextTimeout(drCtx, pollInterval, 30*time.Second, true, func(pollCtx context.Context) (bool, error) {
		if err := cl.Get(pollCtx, client.ObjectKey{Name: "test-postgres-dr", Namespace: namespace}, createdDR); err != nil {
			t.Logf("DataResource not yet available: %v", err)
			return false, nil
		}
		t.Log("DataResource found in cluster")
		return true, nil
	})

	if err != nil {
		t.Errorf("DataResource did not appear within timeout: %v", err)
		return
	}

	// Verify tenant label
	if tenantLabel, exists := createdDR.Labels["nest.penguintech.io/tenant"]; !exists {
		t.Error("tenant label not found on DataResource")
	} else if tenantLabel != "nest" {
		t.Errorf("tenant label is %s, expected nest", tenantLabel)
	} else {
		t.Log("tenant label verified")
	}

	// Delete the DataResource
	if err := cl.Delete(drCtx, dr); err != nil {
		t.Fatalf("failed to delete DataResource: %v", err)
	}
	t.Log("DataResource deletion initiated")

	// Wait for the DataResource to be deleted
	err = wait.PollUntilContextTimeout(drCtx, pollInterval, 30*time.Second, true, func(pollCtx context.Context) (bool, error) {
		if err := cl.Get(pollCtx, client.ObjectKey{Name: "test-postgres-dr", Namespace: namespace}, &nestv1.DataResource{}); err != nil {
			// Object not found is expected
			if strings.Contains(err.Error(), "not found") {
				t.Log("DataResource deleted successfully")
				return true, nil
			}
			return false, nil
		}
		t.Log("DataResource still present, waiting for deletion...")
		return false, nil
	})

	if err != nil {
		t.Errorf("DataResource did not delete within timeout: %v", err)
	}
}
