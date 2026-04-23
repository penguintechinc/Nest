package main

import (
	"flag"
	"os"

	"go.uber.org/zap"

	"github.com/penguintechinc/nest/services/csi/driver"
)

func main() {
	var endpoint string
	var nodeID string
	var driverName string
	flag.StringVar(&endpoint, "endpoint", "unix:///var/lib/kubelet/plugins/csi.nest.penguintech.io/csi.sock", "CSI endpoint")
	flag.StringVar(&nodeID, "node-id", os.Getenv("NODE_NAME"), "Node ID")
	flag.StringVar(&driverName, "driver-name", "csi.nest.penguintech.io", "CSI driver name")
	flag.Parse()

	logger, _ := zap.NewProduction()
	defer logger.Sync()

	logger.Info("Starting Nest CSI driver",
		zap.String("endpoint", endpoint),
		zap.String("nodeID", nodeID),
		zap.String("driverName", driverName),
	)

	d := driver.New(driver.Config{
		Endpoint:   endpoint,
		NodeID:     nodeID,
		DriverName: driverName,
		Logger:     logger,
	})

	if err := d.Run(); err != nil {
		logger.Fatal("CSI driver exited", zap.Error(err))
	}
}
