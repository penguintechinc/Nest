//go:build !xdp || !linux
// +build !xdp !linux

package internal

import (
	"context"
	"go.uber.org/zap"
)

// NetworkMode indicates which networking path is active
type NetworkMode int

const (
	ModeStandard NetworkMode = iota
)

// InitNetwork initializes the network subsystem in standard mode (no XDP)
func InitNetwork(ctx context.Context, logger *zap.Logger) (NetworkMode, error) {
	logger.Info("network subsystem initialized", zap.String("mode", "standard"))
	return ModeStandard, nil
}

// GetNetworkMode returns the active networking mode
func GetNetworkMode() NetworkMode {
	return ModeStandard
}
