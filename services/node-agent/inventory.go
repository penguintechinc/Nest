package main

import (
	"bufio"
	"bytes"
	"context"
	"os/exec"
	"strconv"
	"strings"

	"go.uber.org/zap"
)

// DeviceInfo is the in-process representation of a discovered block device
type DeviceInfo struct {
	Name          string
	Serial        string
	Model         string
	CapacityBytes int64
	Class         string
	State         string
	SMART         *SMARTInfo
	Signature     string
}

// SMARTInfo holds parsed SMART attributes
type SMARTInfo struct {
	Health              string
	WearPercent         int32
	HoursOn             int64
	TemperatureCelsius  int32
	ReallocatedSectors  int64
}

// InventoryCollector collects drive inventory from the OS
type InventoryCollector struct {
	nodeName string
	logger   *zap.Logger
}

func NewInventoryCollector(nodeName string, logger *zap.Logger) *InventoryCollector {
	return &InventoryCollector{nodeName: nodeName, logger: logger}
}

// Collect discovers all block devices on the node and classifies them
func (c *InventoryCollector) Collect(ctx context.Context) ([]*DeviceInfo, error) {
	devices, err := c.lsblk(ctx)
	if err != nil {
		c.logger.Warn("lsblk failed, using stub data", zap.Error(err))
		return c.stubDevices(), nil
	}

	for _, d := range devices {
		d.Class = c.classifyDevice(d)
		d.State = c.detectState(d)
		// SMART data collected daily (or on hot-plug), not every scan
		// For P1: stub SMART data
		d.SMART = c.stubSMART(d)
	}

	return devices, nil
}

// lsblk runs lsblk to discover block devices
func (c *InventoryCollector) lsblk(ctx context.Context) ([]*DeviceInfo, error) {
	cmd := exec.CommandContext(ctx, "lsblk",
		"-d",           // no partitions
		"-n",           // no header
		"-o", "NAME,SERIAL,MODEL,SIZE,ROTA,TYPE",
		"--bytes",
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var devices []*DeviceInfo
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		// Only process disk type
		if fields[5] != "disk" {
			continue
		}
		size, _ := strconv.ParseInt(fields[3], 10, 64)
		devices = append(devices, &DeviceInfo{
			Name:          "/dev/" + fields[0],
			Serial:        fields[1],
			Model:         fields[2],
			CapacityBytes: size,
		})
	}
	return devices, nil
}

// classifyDevice assigns a hardware tier class based on device characteristics
func (c *InventoryCollector) classifyDevice(d *DeviceInfo) string {
	name := strings.ToLower(d.Name)
	model := strings.ToLower(d.Model)

	// NVMe devices are always nvme-hot
	if strings.Contains(name, "nvme") {
		return "nvme-hot"
	}

	// Known SSD models → ssd-warm
	ssdKeywords := []string{"ssd", "solid", "samsung", "crucial", "intel", "micron", "evo", "pro"}
	for _, kw := range ssdKeywords {
		if strings.Contains(model, kw) {
			return "ssd-warm"
		}
	}

	// Large HDDs → sata-cold
	if d.CapacityBytes >= 16*1024*1024*1024*1024 { // 16 TiB
		return "sata-cold"
	}

	// Default: sata-bulk
	return "sata-bulk"
}

// detectState determines if the drive is active (in use) or dark (unallocated)
func (c *InventoryCollector) detectState(d *DeviceInfo) string {
	// P1 stub: check if device has a mount point via lsblk
	// In production: check OSD membership, LVM PV, ZFS pool, mdraid
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "lsblk", "-no", "MOUNTPOINT", d.Name)
	out, err := cmd.Output()
	if err != nil {
		return "Dark"
	}
	if strings.TrimSpace(string(out)) != "" {
		return "Active"
	}
	return "Dark"
}

// stubDevices returns fake devices for environments without real block devices
func (c *InventoryCollector) stubDevices() []*DeviceInfo {
	return []*DeviceInfo{
		{
			Name:          "/dev/stub-nvme0n1",
			Serial:        "STUB-NVMe-001",
			Model:         "Samsung SSD 980 PRO",
			CapacityBytes: 4096 * 1024 * 1024 * 1024,
			Class:         "nvme-hot",
			State:         "Active",
		},
		{
			Name:          "/dev/stub-sda",
			Serial:        "STUB-HDD-001",
			Model:         "WD Ultrastar DC HC550",
			CapacityBytes: 18 * 1024 * 1024 * 1024 * 1024,
			Class:         "sata-cold",
			State:         "Dark",
		},
	}
}

func (c *InventoryCollector) stubSMART(d *DeviceInfo) *SMARTInfo {
	return &SMARTInfo{
		Health:             "PASSED",
		WearPercent:        5,
		HoursOn:            500,
		TemperatureCelsius: 35,
		ReallocatedSectors: 0,
	}
}
