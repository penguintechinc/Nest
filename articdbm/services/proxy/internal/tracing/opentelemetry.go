package tracing

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type TracingManager struct {
	logger   *zap.Logger
	config   TracingConfig

	// OpenTelemetry components
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	tracer         trace.Tracer
	meter          metric.Meter

	// Exporters
	traceExporters []sdktrace.SpanExporter
	metricExporter sdkmetric.Exporter

	// Custom instrumentation
	dbConnections      metric.Int64Counter
	queryDuration      metric.Float64Histogram
	authAttempts       metric.Int64Counter
	securityEvents     metric.Int64Counter
	xdpPacketsProcessed metric.Int64Counter
	threatIntelMatches  metric.Int64Counter

	// Sampling configuration
	sampler            sdktrace.Sampler
	customSamplers     map[string]sdktrace.Sampler

	// Span storage for correlation
	activeSpans map[string]trace.Span
	spanMutex   sync.RWMutex

	// Performance tracking
	tracingOverhead    time.Duration
	lastFlushTime      time.Time
	spanCount          int64
	droppedSpanCount   int64
	exportErrors       int64
}

type TracingConfig struct {
	ServiceName         string                 `yaml:"service_name"`
	ServiceVersion      string                 `yaml:"service_version"`
	Environment         string                 `yaml:"environment"`

	// Trace configuration
	TracingEnabled      bool                   `yaml:"tracing_enabled"`
	SamplingRate        float64                `yaml:"sampling_rate"`
	MaxSpansPerTrace    int                    `yaml:"max_spans_per_trace"`
	SpanTimeout         time.Duration          `yaml:"span_timeout"`

	// Exporters
	Exporters          []ExporterConfig       `yaml:"exporters"`

	// Resource attributes
	ResourceAttributes map[string]string      `yaml:"resource_attributes"`

	// Instrumentation
	DatabaseInstrumentation    bool           `yaml:"database_instrumentation"`
	HTTPInstrumentation        bool           `yaml:"http_instrumentation"`
	SecurityInstrumentation    bool           `yaml:"security_instrumentation"`
	XDPInstrumentation         bool           `yaml:"xdp_instrumentation"`

	// Performance
	BatchTimeout       time.Duration          `yaml:"batch_timeout"`
	BatchSize          int                    `yaml:"batch_size"`
	QueueSize          int                    `yaml:"queue_size"`

	// Custom sampling rules
	SamplingRules      []SamplingRule         `yaml:"sampling_rules"`
}

type ExporterConfig struct {
	Type           string                 `yaml:"type"` // "jaeger", "zipkin", "otlp", "prometheus"
	Endpoint       string                 `yaml:"endpoint"`
	Insecure       bool                   `yaml:"insecure"`
	Headers        map[string]string      `yaml:"headers"`
	Timeout        time.Duration          `yaml:"timeout"`
	Compression    string                 `yaml:"compression"`
	Protocol       string                 `yaml:"protocol"` // "grpc" or "http" for OTLP
	APIKey         string                 `yaml:"api_key,omitempty"`
}

type SamplingRule struct {
	Service        string                 `yaml:"service,omitempty"`
	Operation      string                 `yaml:"operation,omitempty"`
	Attribute      string                 `yaml:"attribute,omitempty"`
	AttributeValue string                 `yaml:"attribute_value,omitempty"`
	SamplingRate   float64                `yaml:"sampling_rate"`
	Priority       int                    `yaml:"priority"`
}

type SpanContext struct {
	TraceID    string                `json:"trace_id"`
	SpanID     string                `json:"span_id"`
	Operation  string                `json:"operation"`
	StartTime  time.Time             `json:"start_time"`
	Duration   time.Duration         `json:"duration,omitempty"`
	Tags       map[string]string     `json:"tags"`
	Logs       []SpanLog             `json:"logs,omitempty"`
	Status     string                `json:"status"`
	Error      string                `json:"error,omitempty"`
	ParentSpan string                `json:"parent_span,omitempty"`
}

type SpanLog struct {
	Timestamp time.Time         `json:"timestamp"`
	Fields    map[string]string `json:"fields"`
}

type DatabaseSpanAttributes struct {
	ConnectionString string
	Database         string
	Table            string
	Operation        string
	Query            string
	QueryHash        string
	RowsAffected     int64
	RowsReturned     int64
	User             string
	ClientIP         string
	Duration         time.Duration
}

type SecuritySpanAttributes struct {
	EventType        string
	ThreatLevel      string
	BlockedIP        string
	AttackVector     string
	UserID           string
	SessionID        string
	RuleMatched      string
	Confidence       float64
	ThreatSource     string
	ResponseAction   string
}

func NewTracingManager(logger *zap.Logger, config TracingConfig) (*TracingManager, error) {
	// Set defaults
	if config.ServiceName == "" {
		config.ServiceName = "articdbm-proxy"
	}
	if config.ServiceVersion == "" {
		config.ServiceVersion = "1.2.0"
	}
	if config.Environment == "" {
		config.Environment = "production"
	}
	if config.SamplingRate == 0 {
		config.SamplingRate = 0.1 // 10% sampling by default
	}
	if config.MaxSpansPerTrace == 0 {
		config.MaxSpansPerTrace = 1000
	}
	if config.SpanTimeout == 0 {
		config.SpanTimeout = 5 * time.Minute
	}
	if config.BatchTimeout == 0 {
		config.BatchTimeout = 5 * time.Second
	}
	if config.BatchSize == 0 {
		config.BatchSize = 512
	}
	if config.QueueSize == 0 {
		config.QueueSize = 2048
	}

	tm := &TracingManager{
		logger:         logger,
		config:         config,
		activeSpans:    make(map[string]trace.Span),
		customSamplers: make(map[string]sdktrace.Sampler),
		lastFlushTime:  time.Now(),
	}

	if !config.TracingEnabled {
		logger.Info("Distributed tracing disabled")
		return tm, nil
	}

	// Initialize OpenTelemetry
	if err := tm.initializeOpenTelemetry(); err != nil {
		return nil, fmt.Errorf("failed to initialize OpenTelemetry: %w", err)
	}

	logger.Info("Distributed tracing initialized",
		zap.String("service", config.ServiceName),
		zap.String("version", config.ServiceVersion),
		zap.Float64("sampling_rate", config.SamplingRate),
		zap.Int("exporters", len(config.Exporters)))

	return tm, nil
}

func (tm *TracingManager) initializeOpenTelemetry() error {
	ctx := context.Background()

	// Create resource
	res, err := tm.createResource()
	if err != nil {
		return fmt.Errorf("failed to create resource: %w", err)
	}

	// Initialize trace exporters
	if err := tm.initializeTraceExporters(ctx); err != nil {
		return fmt.Errorf("failed to initialize trace exporters: %w", err)
	}

	// Create sampler
	tm.sampler = tm.createSampler()

	// Create tracer provider
	tm.tracerProvider = sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(tm.sampler),
		sdktrace.WithSpanProcessor(sdktrace.NewBatchSpanProcessor(
			tm.traceExporters[0], // Use first exporter as primary
			sdktrace.WithBatchTimeout(tm.config.BatchTimeout),
			sdktrace.WithMaxExportBatchSize(tm.config.BatchSize),
			sdktrace.WithMaxQueueSize(tm.config.QueueSize),
		)),
	)

	// Initialize metric exporter
	if err := tm.initializeMetricExporter(ctx); err != nil {
		return fmt.Errorf("failed to initialize metric exporter: %w", err)
	}

	// Create meter provider
	tm.meterProvider = sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(
			tm.metricExporter,
			sdkmetric.WithInterval(30*time.Second),
		)),
	)

	// Set global providers
	otel.SetTracerProvider(tm.tracerProvider)
	otel.SetMeterProvider(tm.meterProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Get tracer and meter
	tm.tracer = tm.tracerProvider.Tracer(
		tm.config.ServiceName,
		trace.WithInstrumentationVersion(tm.config.ServiceVersion),
	)

	tm.meter = tm.meterProvider.Meter(
		tm.config.ServiceName,
		metric.WithInstrumentationVersion(tm.config.ServiceVersion),
	)

	// Initialize custom metrics
	return tm.initializeMetrics()
}

func (tm *TracingManager) createResource() (*resource.Resource, error) {
	attributes := []attribute.KeyValue{
		semconv.ServiceName(tm.config.ServiceName),
		semconv.ServiceVersion(tm.config.ServiceVersion),
		semconv.DeploymentEnvironment(tm.config.Environment),
		semconv.ProcessPID(int(os.Getpid())),
		semconv.ProcessExecutableName("articdbm-proxy"),
		semconv.ProcessRuntimeName("go"),
		semconv.ProcessRuntimeVersion(runtime.Version()),
		semconv.HostName(getHostname()),
		semconv.HostArch(runtime.GOARCH),
		semconv.OSType(runtime.GOOS),
	}

	// Add custom resource attributes
	for key, value := range tm.config.ResourceAttributes {
		attributes = append(attributes, attribute.String(key, value))
	}

	return resource.NewWithAttributes(
		semconv.SchemaURL,
		attributes...,
	)
}

func (tm *TracingManager) initializeTraceExporters(ctx context.Context) error {
	tm.traceExporters = make([]sdktrace.SpanExporter, 0, len(tm.config.Exporters))

	for _, exporterConfig := range tm.config.Exporters {
		var exporter sdktrace.SpanExporter
		var err error

		switch exporterConfig.Type {
		case "jaeger":
			exporter, err = tm.createJaegerExporter(exporterConfig)
		case "zipkin":
			exporter, err = tm.createZipkinExporter(exporterConfig)
		case "otlp":
			exporter, err = tm.createOTLPExporter(ctx, exporterConfig)
		default:
			return fmt.Errorf("unsupported trace exporter type: %s", exporterConfig.Type)
		}

		if err != nil {
			tm.logger.Error("Failed to create trace exporter",
				zap.String("type", exporterConfig.Type),
				zap.Error(err))
			continue
		}

		tm.traceExporters = append(tm.traceExporters, exporter)
		tm.logger.Info("Trace exporter initialized",
			zap.String("type", exporterConfig.Type),
			zap.String("endpoint", exporterConfig.Endpoint))
	}

	if len(tm.traceExporters) == 0 {
		return fmt.Errorf("no trace exporters successfully initialized")
	}

	return nil
}

func (tm *TracingManager) createJaegerExporter(config ExporterConfig) (sdktrace.SpanExporter, error) {
	options := []jaeger.EndpointOption{}

	if config.Endpoint != "" {
		options = append(options, jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(config.Endpoint)))
	}

	return jaeger.New(
		jaeger.WithCollectorEndpoint(options...),
	)
}

func (tm *TracingManager) createZipkinExporter(config ExporterConfig) (sdktrace.SpanExporter, error) {
	return zipkin.New(config.Endpoint)
}

func (tm *TracingManager) createOTLPExporter(ctx context.Context, config ExporterConfig) (sdktrace.SpanExporter, error) {
	if config.Protocol == "grpc" || config.Protocol == "" {
		return tm.createOTLPGRPCExporter(ctx, config)
	} else if config.Protocol == "http" {
		return tm.createOTLPHTTPExporter(ctx, config)
	}

	return nil, fmt.Errorf("unsupported OTLP protocol: %s", config.Protocol)
}

func (tm *TracingManager) createOTLPGRPCExporter(ctx context.Context, config ExporterConfig) (sdktrace.SpanExporter, error) {
	options := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(config.Endpoint),
		otlptracegrpc.WithTimeout(config.Timeout),
	}

	if config.Insecure {
		options = append(options, otlptracegrpc.WithInsecure())
	} else {
		options = append(options, otlptracegrpc.WithTLSCredentials(credentials.NewTLS(&tls.Config{})))
	}

	if len(config.Headers) > 0 {
		options = append(options, otlptracegrpc.WithHeaders(config.Headers))
	}

	if config.Compression != "" {
		options = append(options, otlptracegrpc.WithCompressor(config.Compression))
	}

	client := otlptracegrpc.NewClient(options...)
	return otlptrace.New(ctx, client)
}

func (tm *TracingManager) createOTLPHTTPExporter(ctx context.Context, config ExporterConfig) (sdktrace.SpanExporter, error) {
	options := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(config.Endpoint),
		otlptracehttp.WithTimeout(config.Timeout),
	}

	if config.Insecure {
		options = append(options, otlptracehttp.WithInsecure())
	}

	if len(config.Headers) > 0 {
		options = append(options, otlptracehttp.WithHeaders(config.Headers))
	}

	if config.Compression != "" {
		options = append(options, otlptracehttp.WithCompression(otlptracehttp.GzipCompression))
	}

	client := otlptracehttp.NewClient(options...)
	return otlptrace.New(ctx, client)
}

func (tm *TracingManager) initializeMetricExporter(ctx context.Context) error {
	// For now, use Prometheus as the primary metric exporter
	// In a real implementation, this would be configurable
	var err error
	tm.metricExporter, err = prometheus.New()
	if err != nil {
		return fmt.Errorf("failed to create Prometheus exporter: %w", err)
	}

	return nil
}

func (tm *TracingManager) createSampler() sdktrace.Sampler {
	// Create custom sampler with rules
	if len(tm.config.SamplingRules) > 0 {
		return tm.createRuleBased Sampler()
	}

	// Use probability sampler
	return sdktrace.TraceIDRatioBased(tm.config.SamplingRate)
}

func (tm *TracingManager) createRuleBasedSampler() sdktrace.Sampler {
	// Custom sampler implementation with rules
	return sdktrace.ParentBased(
		sdktrace.TraceIDRatioBased(tm.config.SamplingRate),
	)
}

func (tm *TracingManager) initializeMetrics() error {
	var err error

	// Database connection metrics
	tm.dbConnections, err = tm.meter.Int64Counter(
		"articdbm_db_connections_total",
		metric.WithDescription("Total number of database connections"),
	)
	if err != nil {
		return err
	}

	// Query duration metrics
	tm.queryDuration, err = tm.meter.Float64Histogram(
		"articdbm_query_duration_seconds",
		metric.WithDescription("Database query execution duration in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return err
	}

	// Authentication attempts
	tm.authAttempts, err = tm.meter.Int64Counter(
		"articdbm_auth_attempts_total",
		metric.WithDescription("Total number of authentication attempts"),
	)
	if err != nil {
		return err
	}

	// Security events
	tm.securityEvents, err = tm.meter.Int64Counter(
		"articdbm_security_events_total",
		metric.WithDescription("Total number of security events"),
	)
	if err != nil {
		return err
	}

	// XDP packets processed
	tm.xdpPacketsProcessed, err = tm.meter.Int64Counter(
		"articdbm_xdp_packets_processed_total",
		metric.WithDescription("Total number of XDP packets processed"),
	)
	if err != nil {
		return err
	}

	// Threat intelligence matches
	tm.threatIntelMatches, err = tm.meter.Int64Counter(
		"articdbm_threat_intel_matches_total",
		metric.WithDescription("Total number of threat intelligence matches"),
	)
	if err != nil {
		return err
	}

	return nil
}

func (tm *TracingManager) StartSpan(ctx context.Context, operationName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	if !tm.config.TracingEnabled || tm.tracer == nil {
		return ctx, trace.SpanFromContext(ctx)
	}

	ctx, span := tm.tracer.Start(ctx, operationName, opts...)

	// Store active span for correlation
	tm.spanMutex.Lock()
	tm.activeSpans[span.SpanContext().SpanID().String()] = span
	tm.spanMutex.Unlock()

	tm.spanCount++

	return ctx, span
}

func (tm *TracingManager) StartDatabaseSpan(ctx context.Context, attrs DatabaseSpanAttributes) (context.Context, trace.Span) {
	ctx, span := tm.StartSpan(ctx, fmt.Sprintf("db.%s", attrs.Operation),
		trace.WithAttributes(
			semconv.DBSystemKey.String(attrs.Database),
			semconv.DBName(attrs.Database),
			semconv.DBTableName(attrs.Table),
			semconv.DBOperation(attrs.Operation),
			semconv.DBStatement(attrs.Query),
			attribute.String("db.query_hash", attrs.QueryHash),
			attribute.String("db.user", attrs.User),
			attribute.String("client.ip", attrs.ClientIP),
		),
		trace.WithSpanKind(trace.SpanKindClient),
	)

	// Record database connection metric
	if tm.config.DatabaseInstrumentation {
		tm.dbConnections.Add(ctx, 1,
			metric.WithAttributes(
				attribute.String("database", attrs.Database),
				attribute.String("operation", attrs.Operation),
				attribute.String("user", attrs.User),
			),
		)
	}

	return ctx, span
}

func (tm *TracingManager) FinishDatabaseSpan(span trace.Span, attrs DatabaseSpanAttributes, err error) {
	if span == nil {
		return
	}

	// Add final attributes
	span.SetAttributes(
		attribute.Int64("db.rows_affected", attrs.RowsAffected),
		attribute.Int64("db.rows_returned", attrs.RowsReturned),
		attribute.Float64("duration_ms", float64(attrs.Duration.Nanoseconds())/1e6),
	)

	// Record query duration metric
	if tm.config.DatabaseInstrumentation {
		tm.queryDuration.Record(context.Background(), attrs.Duration.Seconds(),
			metric.WithAttributes(
				attribute.String("database", attrs.Database),
				attribute.String("operation", attrs.Operation),
				attribute.String("table", attrs.Table),
			),
		)
	}

	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	} else {
		span.SetStatus(codes.Ok, "")
	}

	span.End()

	// Remove from active spans
	tm.spanMutex.Lock()
	delete(tm.activeSpans, span.SpanContext().SpanID().String())
	tm.spanMutex.Unlock()
}

func (tm *TracingManager) StartSecuritySpan(ctx context.Context, attrs SecuritySpanAttributes) (context.Context, trace.Span) {
	ctx, span := tm.StartSpan(ctx, fmt.Sprintf("security.%s", attrs.EventType),
		trace.WithAttributes(
			attribute.String("security.event_type", attrs.EventType),
			attribute.String("security.threat_level", attrs.ThreatLevel),
			attribute.String("security.blocked_ip", attrs.BlockedIP),
			attribute.String("security.attack_vector", attrs.AttackVector),
			attribute.String("security.user_id", attrs.UserID),
			attribute.String("security.session_id", attrs.SessionID),
			attribute.String("security.rule_matched", attrs.RuleMatched),
			attribute.Float64("security.confidence", attrs.Confidence),
			attribute.String("security.threat_source", attrs.ThreatSource),
			attribute.String("security.response_action", attrs.ResponseAction),
		),
		trace.WithSpanKind(trace.SpanKindInternal),
	)

	// Record security event metrics
	if tm.config.SecurityInstrumentation {
		tm.securityEvents.Add(ctx, 1,
			metric.WithAttributes(
				attribute.String("event_type", attrs.EventType),
				attribute.String("threat_level", attrs.ThreatLevel),
				attribute.String("response_action", attrs.ResponseAction),
			),
		)

		if attrs.ThreatSource == "threat_intelligence" {
			tm.threatIntelMatches.Add(ctx, 1,
				metric.WithAttributes(
					attribute.String("rule_matched", attrs.RuleMatched),
					attribute.String("threat_level", attrs.ThreatLevel),
				),
			)
		}
	}

	return ctx, span
}

func (tm *TracingManager) RecordAuthAttempt(ctx context.Context, userID, clientIP, result string) {
	if tm.config.SecurityInstrumentation {
		tm.authAttempts.Add(ctx, 1,
			metric.WithAttributes(
				attribute.String("user_id", userID),
				attribute.String("client_ip", clientIP),
				attribute.String("result", result),
			),
		)
	}
}

func (tm *TracingManager) RecordXDPEvent(ctx context.Context, packetsProcessed int64, action string) {
	if tm.config.XDPInstrumentation {
		tm.xdpPacketsProcessed.Add(ctx, packetsProcessed,
			metric.WithAttributes(
				attribute.String("action", action),
			),
		)
	}
}

func (tm *TracingManager) AddSpanEvent(span trace.Span, name string, attributes ...attribute.KeyValue) {
	if span != nil {
		span.AddEvent(name, trace.WithAttributes(attributes...))
	}
}

func (tm *TracingManager) AddSpanAttributes(span trace.Span, attributes ...attribute.KeyValue) {
	if span != nil {
		span.SetAttributes(attributes...)
	}
}

func (tm *TracingManager) GetActiveSpanCount() int {
	tm.spanMutex.RLock()
	defer tm.spanMutex.RUnlock()
	return len(tm.activeSpans)
}

func (tm *TracingManager) GetTracingStatistics() map[string]interface{} {
	tm.spanMutex.RLock()
	activeSpans := len(tm.activeSpans)
	tm.spanMutex.RUnlock()

	return map[string]interface{}{
		"tracing_enabled":      tm.config.TracingEnabled,
		"service_name":         tm.config.ServiceName,
		"sampling_rate":        tm.config.SamplingRate,
		"total_spans":          tm.spanCount,
		"active_spans":         activeSpans,
		"dropped_spans":        tm.droppedSpanCount,
		"export_errors":        tm.exportErrors,
		"tracing_overhead_ms":  float64(tm.tracingOverhead.Nanoseconds()) / 1e6,
		"last_flush":           tm.lastFlushTime,
		"exporters":            len(tm.traceExporters),
	}
}

func (tm *TracingManager) Flush(ctx context.Context) error {
	if tm.tracerProvider == nil {
		return nil
	}

	start := time.Now()
	err := tm.tracerProvider.ForceFlush(ctx)
	tm.tracingOverhead += time.Since(start)
	tm.lastFlushTime = time.Now()

	if err != nil {
		tm.exportErrors++
	}

	return err
}

func (tm *TracingManager) Shutdown(ctx context.Context) error {
	if tm.tracerProvider != nil {
		if err := tm.tracerProvider.Shutdown(ctx); err != nil {
			tm.logger.Error("Failed to shutdown tracer provider", zap.Error(err))
		}
	}

	if tm.meterProvider != nil {
		if err := tm.meterProvider.Shutdown(ctx); err != nil {
			tm.logger.Error("Failed to shutdown meter provider", zap.Error(err))
		}
	}

	tm.logger.Info("Distributed tracing shutdown complete")
	return nil
}

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

// HTTPMiddleware creates OpenTelemetry HTTP middleware
func (tm *TracingManager) HTTPMiddleware(next http.Handler) http.Handler {
	if !tm.config.TracingEnabled || !tm.config.HTTPInstrumentation {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Extract trace context from headers
		ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(r.Header))

		// Start span
		ctx, span := tm.StartSpan(ctx, fmt.Sprintf("HTTP %s %s", r.Method, r.URL.Path),
			trace.WithAttributes(
				semconv.HTTPMethod(r.Method),
				semconv.HTTPURL(r.URL.String()),
				semconv.HTTPUserAgent(r.UserAgent()),
				semconv.HTTPClientIP(getClientIP(r)),
			),
			trace.WithSpanKind(trace.SpanKindServer),
		)
		defer span.End()

		// Create wrapped response writer to capture status code
		wrapped := &wrappedResponseWriter{ResponseWriter: w, statusCode: 200}

		// Call next handler
		next.ServeHTTP(wrapped, r.WithContext(ctx))

		// Add response attributes
		span.SetAttributes(
			semconv.HTTPStatusCode(wrapped.statusCode),
		)

		if wrapped.statusCode >= 400 {
			span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", wrapped.statusCode))
		}
	})
}

type wrappedResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *wrappedResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to remote address
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// CreateSpanFromQuery creates a span specifically for database queries
func (tm *TracingManager) CreateSpanFromQuery(ctx context.Context, query, database, user, clientIP string) (context.Context, trace.Span) {
	operation := extractSQLOperation(query)
	table := extractPrimaryTable(query)
	queryHash := fmt.Sprintf("%x", query)[:16] // Simplified hash

	attrs := DatabaseSpanAttributes{
		Database:  database,
		Table:     table,
		Operation: operation,
		Query:     query,
		QueryHash: queryHash,
		User:      user,
		ClientIP:  clientIP,
	}

	return tm.StartDatabaseSpan(ctx, attrs)
}

func extractSQLOperation(query string) string {
	query = strings.TrimSpace(strings.ToUpper(query))
	if strings.HasPrefix(query, "SELECT") {
		return "SELECT"
	} else if strings.HasPrefix(query, "INSERT") {
		return "INSERT"
	} else if strings.HasPrefix(query, "UPDATE") {
		return "UPDATE"
	} else if strings.HasPrefix(query, "DELETE") {
		return "DELETE"
	}
	return "OTHER"
}

func extractPrimaryTable(query string) string {
	// Simplified table extraction - in a real implementation, use a SQL parser
	query = strings.ToLower(query)

	if strings.Contains(query, "from ") {
		parts := strings.Split(query, "from ")
		if len(parts) > 1 {
			words := strings.Fields(parts[1])
			if len(words) > 0 {
				return words[0]
			}
		}
	}

	if strings.Contains(query, "into ") {
		parts := strings.Split(query, "into ")
		if len(parts) > 1 {
			words := strings.Fields(parts[1])
			if len(words) > 0 {
				return words[0]
			}
		}
	}

	if strings.Contains(query, "update ") {
		parts := strings.Split(query, "update ")
		if len(parts) > 1 {
			words := strings.Fields(parts[1])
			if len(words) > 0 {
				return words[0]
			}
		}
	}

	return "unknown"
}