// Package main implements the Nest iSCSI Gateway service.
// It manages iSCSI target configuration for Ceph RBD-backed block devices
// via the Ceph iSCSI gateway (ceph-iscsi/tcmu-runner).
// Per spec §1.1: iscsi resource type = Ceph RBD via tcmu-runner/LIO.
package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/penguintechinc/nest/services/iscsi-gateway/iscsi"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8083"
	}

	gw := iscsi.New(iscsi.Config{
		CephISCSIEndpoint: getenv("CEPH_ISCSI_ENDPOINT", "http://ceph-iscsi-gw:5000"),
		CephISCSIUser:     os.Getenv("CEPH_ISCSI_USER"),
		CephISCSIPassword: os.Getenv("CEPH_ISCSI_PASSWORD"),
		Logger:            log.Default(),
	})

	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.GET("/ready", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

	targets := r.Group("/api/v1/targets")
	targets.POST("", gw.CreateTarget)
	targets.DELETE("/:targetId", gw.DeleteTarget)
	targets.GET("", gw.ListTargets)
	targets.GET("/:targetId", gw.GetTarget)

	srv := &http.Server{Addr: ":" + port, Handler: r}
	go func() {
		log.Printf("iSCSI gateway listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("iSCSI gateway: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("iSCSI gateway shutting down")
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
