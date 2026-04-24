// Package main implements the Nest NFS Gateway service.
// It manages NFS-Ganesha configuration for CephFS-backed NFS exports,
// generating per-tenant EXPORT blocks and reloading Ganesha configuration.
// Per spec §1.1: nfs resource type = CephFS + NFS-Ganesha.
package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/penguintechinc/nest/services/nfs-gateway/gateway"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	gw := gateway.New(gateway.Config{
		GaneshaConfigPath: getenv("GANESHA_CONFIG_PATH", "/etc/ganesha/exports.d"),
		GaneshaSocketPath: getenv("GANESHA_SOCKET_PATH", "/var/run/ganesha/ganesha.pid"),
		CephFSMount:       getenv("CEPHFS_MOUNT", "/mnt/cephfs"),
		Logger:            log.Default(),
	})

	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.GET("/ready", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

	// Export management API
	exports := r.Group("/api/v1/exports")
	exports.POST("", gw.CreateExport)
	exports.DELETE("/:exportId", gw.DeleteExport)
	exports.GET("", gw.ListExports)
	exports.GET("/:exportId", gw.GetExport)

	srv := &http.Server{Addr: ":" + port, Handler: r}

	go func() {
		log.Printf("NFS gateway listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("NFS gateway: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("NFS gateway shutting down")
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
