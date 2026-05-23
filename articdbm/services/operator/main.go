package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	articdbmv1alpha1 "github.com/penguintechinc/articdbm-operator/api/v1alpha1"
	"github.com/penguintechinc/articdbm-operator/controllers"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(articdbmv1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var webhookPort int
	var xdpEnabled bool
	var maxConcurrentReconciles int

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.IntVar(&webhookPort, "webhook-port", 9443, "Port for admission webhooks")
	flag.BoolVar(&xdpEnabled, "enable-xdp", true, "Enable XDP acceleration for ArticDBM instances")
	flag.IntVar(&maxConcurrentReconciles, "max-concurrent-reconciles", 3, "Maximum concurrent reconciles")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   webhookPort,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "articdbm-operator-leader-election",
		SyncPeriod:            &[]time.Duration{10 * time.Minute}[0],
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Setup ArticDBM controller
	if err = (&controllers.ArticDBMReconciler{
		Client:                  mgr.GetClient(),
		Scheme:                  mgr.GetScheme(),
		XDPEnabled:             xdpEnabled,
		MaxConcurrentReconciles: maxConcurrentReconciles,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ArticDBM")
		os.Exit(1)
	}

	// Setup XDP controller
	if xdpEnabled {
		if err = (&controllers.XDPConfigReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "XDPConfig")
			os.Exit(1)
		}
	}

	// Setup Blue/Green deployment controller
	if err = (&controllers.BlueGreenReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "BlueGreen")
		os.Exit(1)
	}

	// Setup webhooks
	if err = (&articdbmv1alpha1.ArticDBM{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "ArticDBM")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting ArticDBM operator")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// ArticDBM CRD definition
type ArticDBMSpec struct {
	// Proxy configuration
	Proxy ProxySpec `json:"proxy"`

	// XDP configuration
	XDP XDPSpec `json:"xdp,omitempty"`

	// Database backends
	Backends BackendsSpec `json:"backends"`

	// Caching configuration
	Cache CacheSpec `json:"cache,omitempty"`

	// Blue/Green deployment
	BlueGreen BlueGreenSpec `json:"blueGreen,omitempty"`

	// Monitoring
	Monitoring MonitoringSpec `json:"monitoring,omitempty"`

	// Security
	Security SecuritySpec `json:"security"`

	// Resources
	Resources ResourceSpec `json:"resources,omitempty"`
}

type ProxySpec struct {
	Replicas      *int32            `json:"replicas,omitempty"`
	Image         string            `json:"image"`
	Version       string            `json:"version,omitempty"`
	NodeSelector  map[string]string `json:"nodeSelector,omitempty"`
	Tolerations   []interface{}     `json:"tolerations,omitempty"`
	Affinity      *interface{}      `json:"affinity,omitempty"`
	HostNetwork   bool              `json:"hostNetwork,omitempty"`
}

type XDPSpec struct {
	Enabled         bool   `json:"enabled"`
	Interface       string `json:"interface,omitempty"`
	RateLimitPPS    int64  `json:"rateLimitPPS,omitempty"`
	BurstLimit      int32  `json:"burstLimit,omitempty"`
	CacheSize       int32  `json:"cacheSize,omitempty"`
	CacheTTL        int32  `json:"cacheTTL,omitempty"`
	IPBlocklistFile string `json:"ipBlocklistFile,omitempty"`
	NumaOptimized   bool   `json:"numaOptimized,omitempty"`
}

type BackendsSpec struct {
	MySQL      []DatabaseBackend `json:"mysql,omitempty"`
	PostgreSQL []DatabaseBackend `json:"postgresql,omitempty"`
	MSSQL      []DatabaseBackend `json:"mssql,omitempty"`
	MongoDB    []DatabaseBackend `json:"mongodb,omitempty"`
	Redis      []RedisBackend    `json:"redis,omitempty"`
}

type DatabaseBackend struct {
	Name         string            `json:"name"`
	Host         string            `json:"host"`
	Port         int32             `json:"port"`
	Database     string            `json:"database"`
	User         string            `json:"user"`
	PasswordRef  SecretKeySelector `json:"passwordRef"`
	Type         string            `json:"type,omitempty"` // "read" or "write"
	Weight       int32             `json:"weight,omitempty"`
	MaxConns     int32             `json:"maxConns,omitempty"`
	TLS          bool              `json:"tls,omitempty"`
}

type RedisBackend struct {
	Name        string            `json:"name"`
	Endpoints   []string          `json:"endpoints"`
	PasswordRef SecretKeySelector `json:"passwordRef,omitempty"`
	Cluster     bool              `json:"cluster,omitempty"`
	Sentinel    bool              `json:"sentinel,omitempty"`
	MasterName  string            `json:"masterName,omitempty"`
}

type SecretKeySelector struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

type CacheSpec struct {
	Enabled          bool          `json:"enabled"`
	Type             string        `json:"type,omitempty"` // "redis", "memcached"
	Size             string        `json:"size,omitempty"`
	TTL              int32         `json:"ttl,omitempty"`
	AuthValidation   bool          `json:"authValidation"`
	HitCounterBased  bool          `json:"hitCounterBased,omitempty"`
	MinHitsToCache   int32         `json:"minHitsToCache,omitempty"`
	EvictionPolicy   string        `json:"evictionPolicy,omitempty"` // "lru", "lfu", "ttl"
}

type BlueGreenSpec struct {
	Enabled            bool    `json:"enabled"`
	Strategy           string  `json:"strategy,omitempty"` // "canary", "blue-green", "rolling"
	TrafficPercentage  int32   `json:"trafficPercentage,omitempty"`
	AutoPromote        bool    `json:"autoPromote,omitempty"`
	PromoteAfter       string  `json:"promoteAfter,omitempty"`
	RollbackOnFailure  bool    `json:"rollbackOnFailure,omitempty"`
	HealthCheckPath    string  `json:"healthCheckPath,omitempty"`
}

type MonitoringSpec struct {
	Prometheus PrometheusSpec `json:"prometheus,omitempty"`
	Grafana    GrafanaSpec    `json:"grafana,omitempty"`
	Tracing    TracingSpec    `json:"tracing,omitempty"`
}

type PrometheusSpec struct {
	Enabled           bool   `json:"enabled"`
	ServiceMonitor    bool   `json:"serviceMonitor,omitempty"`
	Interval          string `json:"interval,omitempty"`
	ScrapeTimeout     string `json:"scrapeTimeout,omitempty"`
	MetricsPath       string `json:"metricsPath,omitempty"`
}

type GrafanaSpec struct {
	Enabled    bool   `json:"enabled"`
	Dashboards bool   `json:"dashboards,omitempty"`
	Datasource string `json:"datasource,omitempty"`
}

type TracingSpec struct {
	Enabled      bool    `json:"enabled"`
	Provider     string  `json:"provider,omitempty"` // "jaeger", "zipkin", "datadog"
	Endpoint     string  `json:"endpoint,omitempty"`
	SamplingRate float64 `json:"samplingRate,omitempty"`
}

type SecuritySpec struct {
	TLS                   TLSSpec                `json:"tls,omitempty"`
	Authentication        AuthenticationSpec     `json:"authentication"`
	Authorization         AuthorizationSpec      `json:"authorization,omitempty"`
	SQLInjectionDetection bool                   `json:"sqlInjectionDetection"`
	ThreatIntelligence    ThreatIntelligenceSpec `json:"threatIntelligence,omitempty"`
	IPWhitelisting        []string               `json:"ipWhitelisting,omitempty"`
	RateLimiting          RateLimitingSpec       `json:"rateLimiting,omitempty"`
}

type TLSSpec struct {
	Enabled     bool              `json:"enabled"`
	CertRef     SecretKeySelector `json:"certRef,omitempty"`
	KeyRef      SecretKeySelector `json:"keyRef,omitempty"`
	CARef       SecretKeySelector `json:"caRef,omitempty"`
	ClientAuth  bool              `json:"clientAuth,omitempty"`
	MinVersion  string            `json:"minVersion,omitempty"`
}

type AuthenticationSpec struct {
	Type          string            `json:"type"` // "basic", "oauth", "ldap", "saml"
	ConfigRef     SecretKeySelector `json:"configRef,omitempty"`
	APIKeyAuth    bool              `json:"apiKeyAuth,omitempty"`
	MFA           bool              `json:"mfa,omitempty"`
	SessionTimeout string           `json:"sessionTimeout,omitempty"`
}

type AuthorizationSpec struct {
	RBAC           bool   `json:"rbac"`
	PolicyFile     string `json:"policyFile,omitempty"`
	DefaultPolicy  string `json:"defaultPolicy,omitempty"`
}

type ThreatIntelligenceSpec struct {
	Enabled      bool     `json:"enabled"`
	Feeds        []string `json:"feeds,omitempty"`
	UpdateInterval string `json:"updateInterval,omitempty"`
	AutoBlock    bool     `json:"autoBlock,omitempty"`
}

type RateLimitingSpec struct {
	Enabled      bool  `json:"enabled"`
	RequestsPerSecond int32 `json:"requestsPerSecond,omitempty"`
	BurstSize    int32 `json:"burstSize,omitempty"`
	PerUser      bool  `json:"perUser,omitempty"`
	PerIP        bool  `json:"perIP,omitempty"`
}

type ResourceSpec struct {
	Requests ResourceRequirements `json:"requests,omitempty"`
	Limits   ResourceRequirements `json:"limits,omitempty"`
}

type ResourceRequirements struct {
	CPU       string `json:"cpu,omitempty"`
	Memory    string `json:"memory,omitempty"`
	HugePages string `json:"hugePages,omitempty"`
}