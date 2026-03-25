package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	activeConnections = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "articdbm_active_connections",
		Help: "Number of active connections",
	}, []string{"database_type"})

	totalQueries = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "articdbm_total_queries",
		Help: "Total number of queries processed",
	}, []string{"database_type", "query_type"})

	queryDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "articdbm_query_duration_seconds",
		Help:    "Query execution duration",
		Buckets: prometheus.DefBuckets,
	}, []string{"database_type", "query_type"})

	authFailures = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "articdbm_auth_failures_total",
		Help: "Total number of authentication failures",
	}, []string{"database_type"})

	sqlInjectionAttempts = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "articdbm_sql_injection_attempts_total",
		Help: "Total number of SQL injection attempts detected",
	}, []string{"database_type"})

	backendErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "articdbm_backend_errors_total",
		Help: "Total number of backend connection errors",
	}, []string{"database_type", "backend"})

	configReloads = promauto.NewCounter(prometheus.CounterOpts{
		Name: "articdbm_config_reloads_total",
		Help: "Total number of configuration reloads",
	})
)

func InitMetrics() {
	prometheus.MustRegister(
		activeConnections,
		totalQueries,
		queryDuration,
		authFailures,
		sqlInjectionAttempts,
		backendErrors,
		configReloads,
	)
}

func IncConnection(dbType string) {
	activeConnections.WithLabelValues(dbType).Inc()
}

func DecConnection(dbType string) {
	activeConnections.WithLabelValues(dbType).Dec()
}

func IncQuery(dbType string, isWrite bool) {
	queryType := "read"
	if isWrite {
		queryType = "write"
	}
	totalQueries.WithLabelValues(dbType, queryType).Inc()
}

func RecordQueryDuration(dbType string, isWrite bool, duration float64) {
	queryType := "read"
	if isWrite {
		queryType = "write"
	}
	queryDuration.WithLabelValues(dbType, queryType).Observe(duration)
}

func IncAuthFailure(dbType string) {
	authFailures.WithLabelValues(dbType).Inc()
}

func IncSQLInjection(dbType string) {
	sqlInjectionAttempts.WithLabelValues(dbType).Inc()
}

func IncBackendError(dbType, backend string) {
	backendErrors.WithLabelValues(dbType, backend).Inc()
}

func IncConfigReload() {
	configReloads.Inc()
}