package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	var nodeName string
	var scanInterval time.Duration
	var metricsAddr string
	flag.StringVar(&nodeName, "node-name", os.Getenv("NODE_NAME"), "K8s node name (from downward API)")
	flag.DurationVar(&scanInterval, "scan-interval", 5*time.Minute, "Drive inventory scan interval")
	flag.StringVar(&metricsAddr, "metrics-addr", ":9090", "Prometheus metrics address")
	flag.Parse()

	if nodeName == "" {
		nodeName, _ = os.Hostname()
	}

	logLevel := zapcore.InfoLevel
	if os.Getenv("LOG_LEVEL") == "debug" {
		logLevel = zapcore.DebugLevel
	}
	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(logLevel)
	logger, _ := cfg.Build()
	defer logger.Sync()

	logger.Info("Nest node-agent starting",
		zap.String("node", nodeName),
		zap.Duration("scanInterval", scanInterval),
	)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	agent := NewAgent(AgentConfig{
		NodeName:     nodeName,
		ScanInterval: scanInterval,
		MetricsAddr:  metricsAddr,
		Logger:       logger,
	})

	if err := agent.Run(ctx); err != nil && err != context.Canceled {
		logger.Fatal("node-agent exited with error", zap.Error(err))
	}
	logger.Info("node-agent stopped")
}
