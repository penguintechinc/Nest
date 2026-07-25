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

	if err := run(context.Background(), os.Getenv("ADDR")); err != nil {
		logger.Fatal("server error", zap.Error(err))
	}
}

// run starts the webhook server and blocks until context is cancelled.
// It returns an error if the server fails to start or encounters an unrecoverable error.
func run(ctx context.Context, addr string) error {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	if addr == "" {
		addr = ":8443" // HTTPS for mutating webhook
	}

	handler := NewWebhookHandler(logger)
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/mutate/pods", handler.MutatePod)
	mux.HandleFunc("/mutate/pvcs", handler.MutatePVC)

	tlsCert := os.Getenv("TLS_CERT_FILE")
	tlsKey := os.Getenv("TLS_KEY_FILE")

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	serverErr := make(chan error, 1)
	go func() {
		var err error
		if tlsCert != "" && tlsKey != "" {
			logger.Info("starting HTTPS webhook server", zap.String("addr", addr))
			err = srv.ListenAndServeTLS(tlsCert, tlsKey)
		} else {
			logger.Warn("TLS cert/key not set; starting HTTP (not suitable for production webhook)")
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return err
	case <-sigCtx.Done():
	}

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutCtx)
}
