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
					{
						"type":        "pvc/block",
						"description": "ReadWriteOnce block storage backed by Ceph RBD",
						"accessModes": []string{"ReadWriteOnce"},
						"protocols":   []string{"csi", "iscsi"},
						"classes":     []string{"nest-rbd", "nest-longhorn-compat"},
					},
					{
						"type":        "pvc/file",
						"description": "ReadWriteMany file storage backed by CephFS",
						"accessModes": []string{"ReadWriteOnce", "ReadWriteMany", "ReadOnlyMany"},
						"protocols":   []string{"csi", "nfs"},
						"classes":     []string{"nest-cephfs"},
					},
					{
						"type":        "object",
						"description": "S3-compatible object storage backed by Ceph RGW",
						"accessModes": []string{"ReadWrite"},
						"protocols":   []string{"s3", "swift"},
						"classes":     []string{"nest-object"},
					},
					{
						"type":        "nfs",
						"description": "NFSv4 mount backed by CephFS with NFS-Ganesha",
						"accessModes": []string{"ReadWriteMany", "ReadOnlyMany"},
						"protocols":   []string{"nfs"},
						"classes":     []string{"nest-nfs"},
					},
					{
						"type":        "iscsi",
						"description": "iSCSI block storage backed by Ceph RBD via LIO/tcmu-runner",
						"accessModes": []string{"ReadWriteOnce"},
						"protocols":   []string{"iscsi"},
						"classes":     []string{"nest-iscsi"},
					},
					{
						"type":        "postgres",
						"description": "Managed PostgreSQL via CloudNativePG",
						"accessModes": []string{"ReadWrite"},
						"protocols":   []string{"native", "rest"},
						"classes":     []string{"postgres-oltp", "postgres-analytics"},
					},
					{
						"type":        "keyvalue",
						"description": "Valkey (Redis-compatible) key-value store",
						"accessModes": []string{"ReadWrite"},
						"protocols":   []string{"resp"},
						"classes":     []string{"keyvalue-shared", "keyvalue-dedicated"},
					},
				},
				"meta": gin.H{"version": 1},
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
			dr.POST("/:name/snapshot", handlers.SnapshotDataResource(s))
			dr.POST("/:name/restore", handlers.RestoreDataResource(s))

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
