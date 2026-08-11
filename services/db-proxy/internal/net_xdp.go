//go:build xdp && linux

package internal

import (
	"context"
	"os"

	"go.uber.org/zap"
	"golang.org/x/sys/unix"
)

// NetworkMode indicates which networking path is active
type NetworkMode int

const (
	ModeStandard NetworkMode = iota
	ModeXDP
)

var activeNetworkMode NetworkMode = ModeStandard

// InitNetwork initializes the network subsystem, detecting XDP capability at runtime
// If XDP is disabled via env var or capabilities are absent, falls back to standard mode
func InitNetwork(ctx context.Context, logger *zap.Logger) (NetworkMode, error) {
	// Check runtime enable flag
	xdpEnabled := os.Getenv("DBPROXY_XDP_ENABLED") == "true" ||
		os.Getenv("XDP_MODE") == "native"

	if !xdpEnabled {
		logger.Info("XDP disabled via env var",
			zap.String("DBPROXY_XDP_ENABLED", os.Getenv("DBPROXY_XDP_ENABLED")),
		)
		activeNetworkMode = ModeStandard
		return ModeStandard, nil
	}

	// Check for NET_ADMIN capability (required for XDP socket creation)
	if !hasNetAdmin() {
		logger.Warn("NET_ADMIN capability not available, falling back to standard networking")
		activeNetworkMode = ModeStandard
		return ModeStandard, nil
	}

	logger.Info("network subsystem initialized", zap.String("mode", "xdp"))
	activeNetworkMode = ModeXDP
	return ModeXDP, nil
}

// hasNetAdmin probes for NET_ADMIN capability by attempting to create a raw socket
// This is cheaper and more reliable than /proc-based capability checking
func hasNetAdmin() bool {
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW, 0)
	if err != nil {
		return false
	}
	unix.Close(fd)
	return true
}

// GetNetworkMode returns the active networking mode
func GetNetworkMode() NetworkMode {
	return activeNetworkMode
}

// SetNetworkMode allows runtime mode switching (for testing)
func SetNetworkMode(mode NetworkMode) {
	activeNetworkMode = mode
}

// XDPSocketOptions holds XDP socket configuration (placeholder for future implementation)
type XDPSocketOptions struct {
	UMEMSize  uint32
	FrameSize uint32
	RxRings   uint32
	TxRings   uint32
}

// Note: Full XDP/AF_XDP implementation deferred. This stub provides:
// - Runtime capability detection + fallback
// - Mode selection at startup
// - Clean toggle mechanism for testing
// Next phase will implement the actual eBPF program loading and AF_XDP zero-copy path
