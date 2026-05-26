package main

import (
	"flag"
	"os"

	"go.uber.org/zap"

	"github.com/penguintechinc/nest/services/csi/driver"
)

func main() {
	if err := runWithFlags(os.Args[1:]); err != nil {
		logger, _ := zap.NewProduction()
		defer logger.Sync()
		logger.Fatal("CSI driver exited", zap.Error(err))
	}
}

// runWithFlags parses command-line flags and starts the CSI driver.
// It is extracted for testability.
func runWithFlags(args []string) error {
	fs := flag.NewFlagSet("csi-driver", flag.ContinueOnError)
	var endpoint string
	var nodeID string
	var driverName string
	var rookRBDSocket string
	var rookCephFSSocket string
	fs.StringVar(&endpoint, "endpoint", "unix:///var/lib/kubelet/plugins/csi.nest.penguintech.io/csi.sock", "CSI endpoint")
	fs.StringVar(&nodeID, "node-id", os.Getenv("NODE_NAME"), "Node ID")
	fs.StringVar(&driverName, "driver-name", "csi.nest.penguintech.io", "CSI driver name")
	fs.StringVar(&rookRBDSocket, "rook-rbd-socket", "unix:///var/lib/kubelet/plugins/rook-ceph.rbd.csi.ceph.com/csi.sock", "Rook-Ceph RBD CSI socket path")
	fs.StringVar(&rookCephFSSocket, "rook-cephfs-socket", "unix:///var/lib/kubelet/plugins/rook-ceph.cephfs.csi.ceph.com/csi.sock", "Rook-Ceph CephFS CSI socket path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	logger, _ := zap.NewProduction()
	defer logger.Sync()

	logger.Info("Starting Nest CSI driver",
		zap.String("endpoint", endpoint),
		zap.String("nodeID", nodeID),
		zap.String("driverName", driverName),
		zap.String("rookRBDSocket", rookRBDSocket),
		zap.String("rookCephFSSocket", rookCephFSSocket),
	)

	d := driver.New(driver.Config{
		Endpoint:         endpoint,
		NodeID:           nodeID,
		DriverName:       driverName,
		Logger:           logger,
		RookRBDSocket:    rookRBDSocket,
		RookCephFSSocket: rookCephFSSocket,
	})

	return d.Run()
}
