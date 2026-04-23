package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	nestgrpc "github.com/penguintechinc/nest/apps/api/grpc"
	"github.com/penguintechinc/nest/apps/api/handlers"
	"github.com/penguintechinc/nest/apps/api/middleware"
	"github.com/penguintechinc/nest/apps/api/store"
	"github.com/penguintechinc/nest/shared/licensing"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	requestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nest_api_requests_total",
			Help: "Total Nest API requests",
		},
		[]string{"method", "path", "status"},
	)
	requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "nest_api_request_duration_seconds",
			Help:    "Nest API request duration",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
)

func init() {
	prometheus.MustRegister(requestsTotal, requestDuration)
}

func main() {
	licenseClient := licensing.NewClientFromEnv()
	if licenseClient != nil {
		validation, err := licenseClient.Validate()
		if err != nil {
			log.Printf("License validation warning: %v", err)
		} else if !validation.Valid {
			log.Printf("License warning: %s", validation.Message)
		} else {
			log.Printf("License valid for %s (%s tier)", validation.Customer, validation.Tier)
		}
	}

	// Start gRPC server in background before the HTTP server blocks.
	nestgrpc.Start()

	if os.Getenv("GIN_MODE") == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())

	// Prometheus metrics middleware
	r.Use(func(c *gin.Context) {
		timer := prometheus.NewTimer(requestDuration.WithLabelValues(c.Request.Method, c.FullPath()))
		c.Next()
		timer.ObserveDuration()
		requestsTotal.WithLabelValues(
			c.Request.Method,
			c.FullPath(),
			http.StatusText(c.Writer.Status()),
		).Inc()
	})

	// Health + readiness (no auth required)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy", "version": os.Getenv("VERSION")})
	})
	r.GET("/ready", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Authenticated API
	s := store.NewMemoryStore()

	v1 := r.Group("/api/v1")
	v1.Use(middleware.TenantMiddleware())
	{
		// Catalog — available resource types for this tenant
		v1.GET("/catalog", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"types": []gin.H{
					{"type": "postgres", "description": "PostgreSQL database via CloudNativePG"},
					{"type": "object", "description": "S3-compatible object storage via Ceph RGW"},
					{"type": "pvc/block", "description": "Block storage via Ceph RBD"},
					{"type": "pvc/file", "description": "File storage via CephFS"},
					{"type": "keyvalue", "description": "Key-value store (Valkey/Redis-compatible)"},
				},
			})
		})

		// DataResource CRUD — tenant-scoped
		tenants := v1.Group("/tenants/:tenantId")
		{
			// DataResources
			dr := tenants.Group("/data-resources")
			dr.GET("", handlers.ListDataResources(s))
			dr.POST("", handlers.CreateDataResource(s))
			dr.GET("/:name", handlers.GetDataResource(s))
			dr.DELETE("/:name", handlers.DeleteDataResource(s))

			// Operations (LRO)
			tenants.GET("/operations/:opId", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{
					"id":     c.Param("opId"),
					"phase":  "Running",
					"tenant": c.Param("tenantId"),
				})
			})
		}

		// API version discovery
		v1.GET("/versions", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"versions": []gin.H{
					{"version": "v1", "status": "stable", "specUrl": "/api/v1/openapi.json"},
				},
			})
		})
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Nest API server starting on :%s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatal(err)
	}
}
