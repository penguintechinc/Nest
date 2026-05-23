package articdbm

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ArticDBMSpec defines the desired state of ArticDBM
// +kubebuilder:object:root=true
type ArticDBMSpec struct {
	// Proxy defines the proxy deployment settings
	Proxy ProxySpec `json:"proxy"`

	// XDP defines XDP/AF_XDP kernel-level packet processing settings
	XDP XDPSpec `json:"xdp,omitempty"`

	// Backends defines the database backend connections
	Backends BackendsSpec `json:"backends"`

	// Cache defines query result caching settings
	Cache CacheSpec `json:"cache,omitempty"`

	// BlueGreen defines blue/green deployment strategy settings
	BlueGreen BlueGreenSpec `json:"blueGreen,omitempty"`

	// Monitoring defines observability settings
	Monitoring MonitoringSpec `json:"monitoring,omitempty"`

	// Security defines TLS, authentication, and threat protection settings
	Security SecuritySpec `json:"security"`

	// Resources defines CPU/memory/hugepage resource constraints
	Resources ResourceSpec `json:"resources,omitempty"`
}

// ProxySpec defines the proxy deployment settings
type ProxySpec struct {
	// Replicas is the desired number of proxy pods
	// +kubebuilder:default=3
	Replicas *int32 `json:"replicas,omitempty"`

	// Image is the container image for the proxy (without tag)
	Image string `json:"image"`

	// Version is the image tag / release version
	Version string `json:"version,omitempty"`

	// NodeSelector constrains proxy pods to nodes matching these labels
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations allow the proxy pods to be scheduled on tainted nodes
	Tolerations []Toleration `json:"tolerations,omitempty"`

	// HostNetwork enables host networking for the proxy pods (required for XDP)
	HostNetwork bool `json:"hostNetwork,omitempty"`
}

// Toleration mirrors corev1.Toleration without importing the full corev1 package
// at the types layer — the controller imports corev1 directly.
type Toleration struct {
	Key               string `json:"key,omitempty"`
	Operator          string `json:"operator,omitempty"`
	Value             string `json:"value,omitempty"`
	Effect            string `json:"effect,omitempty"`
	TolerationSeconds *int64 `json:"tolerationSeconds,omitempty"`
}

// XDPSpec defines XDP/AF_XDP kernel-level packet processing settings
type XDPSpec struct {
	// Enabled toggles XDP packet processing
	Enabled bool `json:"enabled"`

	// Interface is the network interface to attach XDP programs to
	Interface string `json:"interface,omitempty"`

	// RateLimitPPS is the per-source-IP packet-per-second rate limit
	RateLimitPPS int64 `json:"rateLimitPPS,omitempty"`

	// BurstLimit is the burst allowance above RateLimitPPS
	BurstLimit int32 `json:"burstLimit,omitempty"`

	// CacheSize is the number of entries in the XDP query-result cache
	CacheSize int32 `json:"cacheSize,omitempty"`

	// CacheTTL is the TTL in seconds for cached query results
	CacheTTL int32 `json:"cacheTTL,omitempty"`

	// IPBlocklistFile is the path to a file containing blocked IP prefixes
	IPBlocklistFile string `json:"ipBlocklistFile,omitempty"`

	// NumaOptimized pins XDP worker goroutines and buffer pools to NUMA nodes
	NumaOptimized bool `json:"numaOptimized,omitempty"`
}

// BackendsSpec defines all database backend connection pools
type BackendsSpec struct {
	// MySQL lists MySQL backend servers
	MySQL []DatabaseBackend `json:"mysql,omitempty"`

	// PostgreSQL lists PostgreSQL backend servers
	PostgreSQL []DatabaseBackend `json:"postgresql,omitempty"`

	// MSSQL lists Microsoft SQL Server backend servers
	MSSQL []DatabaseBackend `json:"mssql,omitempty"`

	// MongoDB lists MongoDB backend servers
	MongoDB []DatabaseBackend `json:"mongodb,omitempty"`

	// Redis lists Redis/Valkey backend servers
	Redis []RedisBackend `json:"redis,omitempty"`
}

// DatabaseBackend defines a single relational or document database backend
type DatabaseBackend struct {
	// Name is the unique identifier for this backend within the pool
	Name string `json:"name"`

	// Host is the DNS name or IP of the backend
	Host string `json:"host"`

	// Port is the TCP port of the backend
	Port int32 `json:"port"`

	// Database is the default database/schema name
	Database string `json:"database"`

	// User is the database login username
	User string `json:"user"`

	// PasswordRef references the Kubernetes Secret containing the password
	PasswordRef SecretKeySelector `json:"passwordRef"`

	// Type indicates "read" or "write" role for read/write splitting
	// +kubebuilder:validation:Enum=read;write
	Type string `json:"type,omitempty"`

	// Weight is the relative load-balancing weight for this backend
	// +kubebuilder:default=1
	Weight int32 `json:"weight,omitempty"`

	// MaxConns is the maximum number of connections to hold in the pool
	MaxConns int32 `json:"maxConns,omitempty"`

	// TLS enables TLS for connections to this backend
	TLS bool `json:"tls,omitempty"`
}

// RedisBackend defines a Redis or Valkey backend
type RedisBackend struct {
	// Name is the unique identifier for this Redis backend
	Name string `json:"name"`

	// Endpoints is the list of Redis node addresses (host:port)
	Endpoints []string `json:"endpoints"`

	// PasswordRef optionally references the Secret containing the Redis AUTH password
	PasswordRef SecretKeySelector `json:"passwordRef,omitempty"`

	// Cluster enables Redis Cluster mode
	Cluster bool `json:"cluster,omitempty"`

	// Sentinel enables Redis Sentinel mode
	Sentinel bool `json:"sentinel,omitempty"`

	// MasterName is the Sentinel master name (required when Sentinel=true)
	MasterName string `json:"masterName,omitempty"`
}

// SecretKeySelector identifies a key within a Kubernetes Secret
type SecretKeySelector struct {
	// Name is the name of the Secret
	Name string `json:"name"`

	// Key is the key within the Secret
	Key string `json:"key"`
}

// CacheSpec defines query-result caching behaviour
type CacheSpec struct {
	// Enabled toggles the query result cache
	Enabled bool `json:"enabled"`

	// Type selects the cache backend: "redis" or "memcached"
	// +kubebuilder:validation:Enum=redis;memcached
	Type string `json:"type,omitempty"`

	// Size is the maximum cache size (e.g., "512Mi")
	Size string `json:"size,omitempty"`

	// TTL is the default cache entry TTL in seconds
	TTL int32 `json:"ttl,omitempty"`

	// AuthValidation caches authentication results to reduce backend load
	AuthValidation bool `json:"authValidation"`

	// HitCounterBased only caches queries that have been seen MinHitsToCache times
	HitCounterBased bool `json:"hitCounterBased,omitempty"`

	// MinHitsToCache is the minimum hit count before a query result is cached
	MinHitsToCache int32 `json:"minHitsToCache,omitempty"`

	// EvictionPolicy controls cache eviction: "lru", "lfu", or "ttl"
	// +kubebuilder:validation:Enum=lru;lfu;ttl
	EvictionPolicy string `json:"evictionPolicy,omitempty"`
}

// BlueGreenSpec defines blue/green and canary deployment settings
type BlueGreenSpec struct {
	// Enabled activates blue/green deployment management
	Enabled bool `json:"enabled"`

	// Strategy selects the rollout strategy: "canary", "blue-green", or "rolling"
	// +kubebuilder:validation:Enum=canary;blue-green;rolling
	Strategy string `json:"strategy,omitempty"`

	// TrafficPercentage is the percentage of traffic routed to the new (green) version
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	TrafficPercentage int32 `json:"trafficPercentage,omitempty"`

	// AutoPromote automatically promotes green to 100% when healthy
	AutoPromote bool `json:"autoPromote,omitempty"`

	// PromoteAfter is the duration to wait before auto-promotion (e.g., "10m")
	PromoteAfter string `json:"promoteAfter,omitempty"`

	// RollbackOnFailure triggers automatic rollback when health checks fail
	RollbackOnFailure bool `json:"rollbackOnFailure,omitempty"`

	// HealthCheckPath is the HTTP path used to assess the new version's health
	HealthCheckPath string `json:"healthCheckPath,omitempty"`
}

// MonitoringSpec defines observability settings
type MonitoringSpec struct {
	// Prometheus configures Prometheus metrics scraping
	Prometheus PrometheusSpec `json:"prometheus,omitempty"`

	// Grafana configures Grafana dashboard provisioning
	Grafana GrafanaSpec `json:"grafana,omitempty"`

	// Tracing configures distributed tracing
	Tracing TracingSpec `json:"tracing,omitempty"`
}

// PrometheusSpec configures Prometheus scraping
type PrometheusSpec struct {
	// Enabled toggles Prometheus metrics exposure
	Enabled bool `json:"enabled"`

	// ServiceMonitor creates a Prometheus Operator ServiceMonitor resource
	ServiceMonitor bool `json:"serviceMonitor,omitempty"`

	// Interval is the Prometheus scrape interval (e.g., "30s")
	Interval string `json:"interval,omitempty"`

	// ScrapeTimeout is the Prometheus scrape timeout (e.g., "10s")
	ScrapeTimeout string `json:"scrapeTimeout,omitempty"`

	// MetricsPath is the HTTP path that exposes metrics
	// +kubebuilder:default=/metrics
	MetricsPath string `json:"metricsPath,omitempty"`
}

// GrafanaSpec configures Grafana dashboard provisioning
type GrafanaSpec struct {
	// Enabled toggles Grafana dashboard provisioning
	Enabled bool `json:"enabled"`

	// Dashboards provisions pre-built ArticDBM dashboards into Grafana
	Dashboards bool `json:"dashboards,omitempty"`

	// Datasource is the name of the Grafana datasource to use
	Datasource string `json:"datasource,omitempty"`
}

// TracingSpec configures distributed tracing
type TracingSpec struct {
	// Enabled toggles distributed tracing
	Enabled bool `json:"enabled"`

	// Provider selects the tracing backend: "jaeger", "zipkin", or "datadog"
	// +kubebuilder:validation:Enum=jaeger;zipkin;datadog
	Provider string `json:"provider,omitempty"`

	// Endpoint is the collector endpoint URL
	Endpoint string `json:"endpoint,omitempty"`

	// SamplingRate is the fraction of traces to sample (0.0–1.0)
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1
	SamplingRate float64 `json:"samplingRate,omitempty"`
}

// SecuritySpec defines security posture for the ArticDBM instance
type SecuritySpec struct {
	// TLS configures TLS termination and mTLS settings
	TLS TLSSpec `json:"tls,omitempty"`

	// Authentication configures client authentication
	Authentication AuthenticationSpec `json:"authentication"`

	// Authorization configures RBAC policy enforcement
	Authorization AuthorizationSpec `json:"authorization,omitempty"`

	// SQLInjectionDetection enables real-time SQL injection detection
	SQLInjectionDetection bool `json:"sqlInjectionDetection"`

	// ThreatIntelligence configures threat-feed-based IP blocking
	ThreatIntelligence ThreatIntelligenceSpec `json:"threatIntelligence,omitempty"`

	// IPWhitelisting restricts proxy access to these source IP CIDRs
	IPWhitelisting []string `json:"ipWhitelisting,omitempty"`

	// RateLimiting configures application-level request rate limiting
	RateLimiting RateLimitingSpec `json:"rateLimiting,omitempty"`
}

// TLSSpec configures TLS/mTLS
type TLSSpec struct {
	// Enabled toggles TLS termination
	Enabled bool `json:"enabled"`

	// CertRef references the Secret key containing the TLS certificate
	CertRef SecretKeySelector `json:"certRef,omitempty"`

	// KeyRef references the Secret key containing the TLS private key
	KeyRef SecretKeySelector `json:"keyRef,omitempty"`

	// CARef references the Secret key containing the CA certificate bundle
	CARef SecretKeySelector `json:"caRef,omitempty"`

	// ClientAuth enables mutual TLS (mTLS) client certificate verification
	ClientAuth bool `json:"clientAuth,omitempty"`

	// MinVersion sets the minimum TLS version: "TLS12" or "TLS13"
	// +kubebuilder:validation:Enum=TLS12;TLS13
	// +kubebuilder:default=TLS12
	MinVersion string `json:"minVersion,omitempty"`
}

// AuthenticationSpec configures client authentication
type AuthenticationSpec struct {
	// Type selects the authentication mechanism: "basic", "oauth", "ldap", or "saml"
	// +kubebuilder:validation:Enum=basic;oauth;ldap;saml
	Type string `json:"type"`

	// ConfigRef references the Secret containing provider-specific auth configuration
	ConfigRef SecretKeySelector `json:"configRef,omitempty"`

	// APIKeyAuth enables API key authentication in addition to the primary type
	APIKeyAuth bool `json:"apiKeyAuth,omitempty"`

	// MFA enables multi-factor authentication enforcement
	MFA bool `json:"mfa,omitempty"`

	// SessionTimeout is the idle session timeout duration (e.g., "30m")
	SessionTimeout string `json:"sessionTimeout,omitempty"`
}

// AuthorizationSpec configures RBAC policy enforcement
type AuthorizationSpec struct {
	// RBAC enables role-based access control enforcement
	RBAC bool `json:"rbac"`

	// PolicyFile is the path to the OPA/Casbin policy file
	PolicyFile string `json:"policyFile,omitempty"`

	// DefaultPolicy sets the default decision when no rule matches: "allow" or "deny"
	// +kubebuilder:validation:Enum=allow;deny
	// +kubebuilder:default=deny
	DefaultPolicy string `json:"defaultPolicy,omitempty"`
}

// ThreatIntelligenceSpec configures threat-feed-based IP blocking
type ThreatIntelligenceSpec struct {
	// Enabled toggles threat intelligence feed integration
	Enabled bool `json:"enabled"`

	// Feeds is the list of threat intelligence feed URLs
	Feeds []string `json:"feeds,omitempty"`

	// UpdateInterval is the feed refresh interval (e.g., "1h")
	UpdateInterval string `json:"updateInterval,omitempty"`

	// AutoBlock automatically blocks IPs found in feeds via XDP
	AutoBlock bool `json:"autoBlock,omitempty"`
}

// RateLimitingSpec configures application-level rate limiting
type RateLimitingSpec struct {
	// Enabled toggles rate limiting
	Enabled bool `json:"enabled"`

	// RequestsPerSecond is the sustained request rate limit
	RequestsPerSecond int32 `json:"requestsPerSecond,omitempty"`

	// BurstSize is the burst allowance above RequestsPerSecond
	BurstSize int32 `json:"burstSize,omitempty"`

	// PerUser applies the rate limit per authenticated user identity
	PerUser bool `json:"perUser,omitempty"`

	// PerIP applies the rate limit per source IP address
	PerIP bool `json:"perIP,omitempty"`
}

// ResourceSpec defines CPU, memory, and hugepage resource constraints
type ResourceSpec struct {
	// Requests sets the minimum guaranteed resources
	Requests ResourceRequirements `json:"requests,omitempty"`

	// Limits sets the maximum allowed resources
	Limits ResourceRequirements `json:"limits,omitempty"`
}

// ResourceRequirements holds CPU, memory, and hugepage quantities as strings
// (e.g., "500m", "2Gi", "1Gi") to avoid importing resource.Quantity here.
type ResourceRequirements struct {
	// CPU is a Kubernetes CPU quantity string (e.g., "500m", "2")
	CPU string `json:"cpu,omitempty"`

	// Memory is a Kubernetes memory quantity string (e.g., "256Mi", "2Gi")
	Memory string `json:"memory,omitempty"`

	// HugePages is a hugepages-2Mi quantity string (e.g., "1Gi"); only used when XDP is enabled
	HugePages string `json:"hugePages,omitempty"`
}

// ArticDBMStatus defines the observed state of ArticDBM
type ArticDBMStatus struct {
	// Phase is the high-level lifecycle phase: Pending, Running, Degraded, Failed
	// +kubebuilder:validation:Enum=Pending;Running;Degraded;Failed
	Phase string `json:"phase,omitempty"`

	// ReadyReplicas is the number of proxy pods currently ready
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// Conditions contains detailed status conditions following the Kubernetes API convention
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastReconcileTime is the timestamp of the most recent successful reconciliation
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`

	// BackendCount is the total number of configured backend endpoints across all DB types
	BackendCount int32 `json:"backendCount,omitempty"`

	// XDPStatus describes the current XDP program attachment state
	XDPStatus string `json:"xdpStatus,omitempty"`

	// ActiveVersion is the image version currently serving production traffic
	ActiveVersion string `json:"activeVersion,omitempty"`

	// BlueGreenPhase describes the current blue/green rollout phase
	BlueGreenPhase string `json:"blueGreenPhase,omitempty"`
}

// ArticDBM is the Schema for the articdbms API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=adbm,categories=articdbm
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=".status.readyReplicas"
// +kubebuilder:printcolumn:name="Backends",type=integer,JSONPath=".status.backendCount"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"
type ArticDBM struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ArticDBMSpec   `json:"spec,omitempty"`
	Status ArticDBMStatus `json:"status,omitempty"`
}

// ArticDBMList contains a list of ArticDBM
// +kubebuilder:object:root=true
type ArticDBMList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ArticDBM `json:"items"`
}
