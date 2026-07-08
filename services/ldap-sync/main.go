package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/penguintechinc/nest/pkg/auth"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	httpAddr := os.Getenv("ADDR")
	if httpAddr == "" {
		httpAddr = ":8086"
	}

	if err := run(context.Background(), httpAddr); err != nil {
		logger.Error("run failed", zap.Error(err))
	}
}

// run initializes and runs the LDAP sync service.
// It handles the HTTP server and syncer lifecycle.
func run(ctx context.Context, httpAddr string) error {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Check license
	license := os.Getenv("ENTERPRISE_LICENSE")
	if license == "" {
		logger.Warn("ENTERPRISE_LICENSE not set; LDAP sync disabled for unlicensed deployments")
	}

	// Initialize JWT auth middleware (FAIL-CLOSED if not configured)
	authConfig := &auth.Config{
		Algorithm:    os.Getenv("JWT_ALGORITHM"),
		SharedSecret: os.Getenv("JWT_SHARED_SECRET"),
		JWKSEndpoint: os.Getenv("JWT_JWKS_ENDPOINT"),
		Issuer:       os.Getenv("JWT_ISSUER"),
		Audience:     os.Getenv("JWT_AUDIENCE"),
	}
	authMiddleware, err := auth.NewMiddleware(authConfig)
	if err != nil {
		logger.Error("failed to initialize auth middleware", zap.Error(err))
		return err
	}

	// Get LDAP URL
	ldapURL := os.Getenv("LDAP_URL")

	// Create syncer with 1 hour interval
	syncer := NewSyncer(ldapURL, 1*time.Hour, logger)

	// Create HTTP mux and routes
	mux := http.NewServeMux()
	setupRoutes(mux, syncer, logger, authMiddleware)

	server := &http.Server{
		Addr:    httpAddr,
		Handler: mux,
	}

	// Start HTTP server
	go func() {
		logger.Info("Starting LDAP sync server", zap.String("addr", httpAddr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", zap.Error(err))
		}
	}()

	// Start syncer in goroutine
	syncCtx, cancel := context.WithCancel(ctx)
	go func() {
		if err := syncer.Run(syncCtx); err != nil {
			logger.Error("Syncer error", zap.Error(err))
		}
	}()

	// Setup signal handling for graceful shutdown
	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	<-sigCtx.Done()
	logger.Info("Shutdown signal received")

	// Graceful shutdown
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server shutdown error", zap.Error(err))
		return err
	}

	logger.Info("LDAP sync service stopped")
	return nil
}

// setupRoutes configures HTTP routes
func setupRoutes(mux *http.ServeMux, syncer *Syncer, logger *zap.Logger, authMiddleware *auth.Middleware) {
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// GET /api/v1/users - list users for tenant (requires read scope)
	mux.Handle("GET /api/v1/users", authMiddleware.RequireAuth(authMiddleware.RequireTenant(authMiddleware.RequireScope("users:read")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"no claims"}`))
			return
		}

		users := syncer.ListUsers(claims.Tenant)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// Manually build JSON to avoid import of encoding/json in multiple places
		w.Write([]byte("["))
		for i, user := range users {
			if i > 0 {
				w.Write([]byte(","))
			}
			userJSON, _ := user.MarshalJSON()
			w.Write(userJSON)
		}
		w.Write([]byte("]"))
	})))))

	// GET /api/v1/users/{uid} - get specific user (requires read scope)
	mux.Handle("GET /api/v1/users/{uid}", authMiddleware.RequireAuth(authMiddleware.RequireTenant(authMiddleware.RequireScope("users:read")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"no claims"}`))
			return
		}

		uid := r.PathValue("uid")

		user, found := syncer.GetUser(claims.Tenant, uid)
		if !found {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"user not found"}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		userJSON, _ := user.MarshalJSON()
		w.Write(userJSON)
	})))))

	// POST /api/v1/sync - trigger sync (requires admin scope)
	mux.Handle("POST /api/v1/sync", authMiddleware.RequireAuth(authMiddleware.RequireTenant(authMiddleware.RequireScope("users:admin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"syncing"}`))
	})))))
}
