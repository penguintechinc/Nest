package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	// Get LDAP URL
	ldapURL := os.Getenv("LDAP_URL")

	// Create syncer with 1 hour interval
	syncer := NewSyncer(ldapURL, 1*time.Hour, logger)

	// Create HTTP mux and routes
	mux := http.NewServeMux()
	setupRoutes(mux, syncer, logger)

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
func setupRoutes(mux *http.ServeMux, syncer *Syncer, logger *zap.Logger) {
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		// License check
		license := os.Getenv("ENTERPRISE_LICENSE")
		if license == "" {
			w.WriteHeader(http.StatusPaymentRequired)
			w.Write([]byte(`{"error":"license required"}`))
			return
		}

		if r.Method == http.MethodGet {
			users := syncer.ListUsers()
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
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		// License check
		license := os.Getenv("ENTERPRISE_LICENSE")
		if license == "" {
			w.WriteHeader(http.StatusPaymentRequired)
			w.Write([]byte(`{"error":"license required"}`))
			return
		}

		if r.Method == http.MethodGet {
			// Extract UID from path: /api/v1/users/{uid}
			uid := r.URL.Path[len("/api/v1/users/"):]

			user, found := syncer.GetUser(uid)
			if !found {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"error":"user not found"}`))
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			userJSON, _ := user.MarshalJSON()
			w.Write(userJSON)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/v1/sync", func(w http.ResponseWriter, r *http.Request) {
		// License check
		license := os.Getenv("ENTERPRISE_LICENSE")
		if license == "" {
			w.WriteHeader(http.StatusPaymentRequired)
			w.Write([]byte(`{"error":"license required"}`))
			return
		}

		if r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"syncing"}`))
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}
