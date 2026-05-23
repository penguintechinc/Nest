package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// CommandRunner abstracts exec.Command so that callers can be tested without
// spawning real system processes.
type CommandRunner interface {
	// Run executes the named command with args and returns combined stdout/stderr.
	// Returns (nil, exec.ErrNotFound) when the binary does not exist.
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
	// Run executes without capturing output; returns an error on non-zero exit.
	Run(ctx context.Context, name string, args ...string) error
}

// FileReader abstracts reading arbitrary files so that callers can be tested
// without touching the real filesystem.
type FileReader interface {
	ReadFile(name string) ([]byte, error)
}

// osCommandRunner is the production CommandRunner that delegates to os/exec.
type osCommandRunner struct{}

func (osCommandRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

func (osCommandRunner) Run(ctx context.Context, name string, args ...string) error {
	return exec.CommandContext(ctx, name, args...).Run()
}

// osFileReader is the production FileReader that reads from the real filesystem.
type osFileReader struct{}

func (osFileReader) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

// smartctlOutput is a partial representation of the smartctl --json -a output.
type smartctlOutput struct {
	SMARTStatus struct {
		Passed bool `json:"passed"`
	} `json:"smart_status"`
	Temperature struct {
		Current int32 `json:"current"`
	} `json:"temperature"`
	PowerOnTime struct {
		Hours int64 `json:"hours"`
	} `json:"power_on_time"`
	ATASMARTAttributes struct {
		Table []struct {
			ID  int `json:"id"`
			Raw struct {
				Value int64 `json:"value"`
			} `json:"raw"`
		} `json:"table"`
	} `json:"ata_smart_attributes"`
	NVMeSMARTHealthLog struct {
		PercentageUsed int32 `json:"percentage_used"`
	} `json:"nvme_smart_health_information_log"`
}

// BlockStats holds block-layer I/O statistics parsed from /proc/diskstats.
type BlockStats struct {
	ReadBytes  int64
	WriteBytes int64
	ReadIops   int64 // reads completed
	WriteIops  int64 // writes completed
	InFlight   int64 // io_in_progress (approximates queue depth)
}

// DeviceInfo is the in-process representation of a discovered block device
type DeviceInfo struct {
	Name          string
	Serial        string
	Model         string
	CapacityBytes int64
	Class         string
	State         string
	SMART         *SMARTInfo
	BlockStats    *BlockStats
	Signature     string
}

// SMARTInfo holds parsed SMART attributes
type SMARTInfo struct {
	Health             string
	WearPercent        int32
	HoursOn            int64
	TemperatureCelsius int32
	ReallocatedSectors int64
}

// InventoryCollector collects drive inventory from the OS
type InventoryCollector struct {
	nodeName      string
	logger        *zap.Logger
	mu            sync.Mutex
	lastSMARTScan map[string]time.Time  // keyed by device Name; tracks last SMART scan time
	lastSMART     map[string]*SMARTInfo // keyed by device Name; cached SMART info
	cmd           CommandRunner
	fs            FileReader
}

func NewInventoryCollector(nodeName string, logger *zap.Logger) *InventoryCollector {
	return &InventoryCollector{
		nodeName:      nodeName,
		logger:        logger,
		lastSMARTScan: make(map[string]time.Time),
		lastSMART:     make(map[string]*SMARTInfo),
		cmd:           osCommandRunner{},
		fs:            osFileReader{},
	}
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
		d.State = c.detectState(ctx, d)
		d.SMART = c.collectSMARTWithScheduling(ctx, d.Name)
		d.BlockStats = c.collectBlockStats(d.Name)
	}

	return devices, nil
}

// lsblk runs lsblk to discover block devices
func (c *InventoryCollector) lsblk(ctx context.Context) ([]*DeviceInfo, error) {
	out, err := c.cmd.Output(ctx, "lsblk",
		"-d", // no partitions
		"-n", // no header
		"-o", "NAME,SERIAL,MODEL,SIZE,ROTA,TYPE",
		"--bytes",
	)
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

// detectState determines if the drive is active (in use), dark (unallocated), or system
func (c *InventoryCollector) detectState(ctx context.Context, d *DeviceInfo) string {
	// Check for system mount points (/, /boot, swap, etc.)
	if c.hasSystemMount(ctx, d.Name) {
		d.Signature = "system"
		return "System"
	}

	// Detect filesystem signature (blank, nest-previous, or foreign-fs)
	sig := c.detectSignature(ctx, d.Name)
	d.Signature = sig

	// Dark state: blank, nest-previous, or foreign filesystem (needs erase confirmation)
	if sig == "blank" || sig == "nest-previous" || strings.HasPrefix(sig, "foreign-fs:") {
		return "Dark"
	}

	// Active state: LVM PV, mdraid, ZFS pool member in active use
	if c.isActiveMember(ctx, d.Name) {
		return "Active"
	}

	// Default to Dark if unknown
	return "Dark"
}

// hasSystemMount reads /proc/mounts and checks if devName or any of its partitions
// map to a system mount point or swap.
func (c *InventoryCollector) hasSystemMount(ctx context.Context, devName string) bool {
	data, err := c.fs.ReadFile("/proc/mounts")
	if err != nil {
		c.logger.Debug("cannot read /proc/mounts", zap.Error(err))
		return false
	}

	// System mount points to check
	systemPaths := map[string]bool{
		"/":         true,
		"/boot":     true,
		"/boot/efi": true,
		"/var":      true,
		"/var/lib":  true,
		"/home":     true,
		"/usr":      true,
		"/tmp":      true,
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}

		dev := fields[0]
		mountPoint := fields[1]
		fsType := fields[2]

		// Check direct device match
		if dev == devName {
			// Swap check
			if fsType == "swap" || mountPoint == "none" {
				return true
			}
			// System mount check
			if systemPaths[mountPoint] {
				return true
			}
			// Prefix check for /var/lib, /boot, etc.
			for sys := range systemPaths {
				if strings.HasPrefix(mountPoint, sys+"/") {
					return true
				}
			}
		}

		// Check partition match (e.g., /dev/sda1 is a partition of /dev/sda)
		if strings.HasPrefix(dev, devName) && len(dev) > len(devName) {
			// Ensure it's actually a partition (followed by digit or 'p')
			suffix := dev[len(devName):]
			if len(suffix) > 0 && (suffix[0] == 'p' || (suffix[0] >= '0' && suffix[0] <= '9')) {
				if fsType == "swap" || mountPoint == "none" {
					return true
				}
				if systemPaths[mountPoint] {
					return true
				}
				for sys := range systemPaths {
					if strings.HasPrefix(mountPoint, sys+"/") {
						return true
					}
				}
			}
		}
	}

	return false
}

// detectSignature runs blkid -o export on devName and returns:
//   "blank"              — no filesystem signature
//   "nest-previous"      — previously formatted by Nest (btrfs/zfs with nest label, or Ceph OSD)
//   "foreign-fs:<type>"  — existing filesystem (ext4, xfs, etc.)
func (c *InventoryCollector) detectSignature(ctx context.Context, devName string) string {
	out, err := c.cmd.Output(ctx, "blkid", "-o", "export", devName)
	if err != nil {
		// blkid returns error for blank devices — treat as blank
		return "blank"
	}

	// Parse key=value output
	metadata := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			metadata[parts[0]] = parts[1]
		}
	}

	// Check for Nest labels (btrfs/ZFS with nest.penguintech.io label)
	if label := metadata["LABEL"]; label != "" {
		if strings.Contains(label, "nest.penguintech.io") || strings.Contains(label, "nest-") {
			return "nest-previous"
		}
	}

	// Check for Ceph BlueStore or Nest LVM
	if fsType := metadata["TYPE"]; fsType != "" {
		if fsType == "ceph_bluestore" {
			return "nest-previous"
		}
		if fsType == "LVM2_member" {
			// Check if it's a Ceph volume group
			if c.isCephVG(ctx, devName) {
				return "nest-previous"
			}
		}
		// Any other filesystem type is foreign
		return "foreign-fs:" + fsType
	}

	// Check for partition table only (blank filesystem, but has partitions)
	if ptType := metadata["PTTYPE"]; ptType != "" {
		return "blank"
	}

	// No signature at all — completely blank
	return "blank"
}

// isCephVG checks if devName is an LVM PV in a Ceph volume group
func (c *InventoryCollector) isCephVG(ctx context.Context, devName string) bool {
	// Run pvdisplay --noheadings -C -o vg_name <dev>
	out, err := c.cmd.Output(ctx, "pvdisplay", "--noheadings", "-C", "-o", "vg_name", devName)
	if err != nil {
		return false
	}
	vgName := strings.TrimSpace(string(out))
	return strings.HasPrefix(vgName, "ceph-")
}

// isActiveMember checks if devName is an active LVM PV, mdraid member, or ZFS pool member
func (c *InventoryCollector) isActiveMember(ctx context.Context, devName string) bool {
	// Check LVM PV membership
	if c.isActiveLVMPV(ctx, devName) {
		return true
	}
	// Check mdraid membership
	if c.isActiveMDRaid(ctx, devName) {
		return true
	}
	// Check ZFS pool membership
	if c.isActiveZFSMember(ctx, devName) {
		return true
	}
	return false
}

// isActiveLVMPV checks if devName is an LVM physical volume
func (c *InventoryCollector) isActiveLVMPV(ctx context.Context, devName string) bool {
	out, err := c.cmd.Output(ctx, "pvdisplay", "--noheadings", "-C", "-o", "vg_name", devName)
	if err != nil {
		return false
	}
	vgName := strings.TrimSpace(string(out))
	return vgName != ""
}

// isActiveMDRaid checks if devName is an active mdraid member
func (c *InventoryCollector) isActiveMDRaid(ctx context.Context, devName string) bool {
	out, err := c.cmd.Output(ctx, "mdadm", "--examine", devName)
	if err != nil {
		return false
	}
	// Any output from mdadm --examine indicates membership in an array
	return len(strings.TrimSpace(string(out))) > 0
}

// isActiveZFSMember checks if devName is an active member of a ZFS pool
func (c *InventoryCollector) isActiveZFSMember(ctx context.Context, devName string) bool {
	out, err := c.cmd.Output(ctx, "zpool", "status")
	if err != nil {
		return false
	}
	// Check if devName appears in zpool status output
	return strings.Contains(string(out), devName)
}

// collectSMARTWithScheduling runs smartctl only if the device is new or if the last scan was >23 hours ago.
// Otherwise, returns the cached SMART info. This reduces system load from repeated smartctl invocations.
func (c *InventoryCollector) collectSMARTWithScheduling(ctx context.Context, devName string) *SMARTInfo {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	lastScan, exists := c.lastSMARTScan[devName]

	// If device is new (not in lastSMARTScan), or if last scan was >23 hours ago, run smartctl
	if !exists || now.Sub(lastScan) > 23*time.Hour {
		smart := c.collectSMART(ctx, devName)
		c.lastSMARTScan[devName] = now
		c.lastSMART[devName] = smart
		return smart
	}

	// Device exists and was scanned recently; return cached result
	if cached, ok := c.lastSMART[devName]; ok {
		c.logger.Debug("using cached SMART data",
			zap.String("device", devName),
			zap.Duration("cacheAge", now.Sub(lastScan)))
		return cached
	}

	// Fallback (should not happen if cache is consistent)
	return c.stubSMART(nil)
}

// collectSMART runs smartctl --json -a on devName and parses the output.
// Falls back to the stub if smartctl is unavailable or returns an error.
func (c *InventoryCollector) collectSMART(ctx context.Context, devName string) *SMARTInfo {
	// Extract the bare device name (last path component).
	parts := strings.Split(strings.TrimPrefix(devName, "/dev/"), "/")
	bare := "/dev/" + parts[len(parts)-1]

	out, err := c.cmd.Output(ctx, "smartctl", "--json", "-a", bare)
	if err != nil {
		// smartctl not installed or failed — use stub
		c.logger.Debug("smartctl unavailable, using stub SMART data",
			zap.String("device", bare), zap.Error(err))
		return c.stubSMART(nil)
	}

	var s smartctlOutput
	if jsonErr := json.Unmarshal(out, &s); jsonErr != nil {
		c.logger.Warn("failed to parse smartctl output",
			zap.String("device", bare), zap.Error(jsonErr))
		return c.stubSMART(nil)
	}

	health := "PASSED"
	if !s.SMARTStatus.Passed {
		health = "FAILED"
	}

	// Reallocated sector count is ATA attribute ID 5.
	var reallocated int64
	for _, attr := range s.ATASMARTAttributes.Table {
		if attr.ID == 5 {
			reallocated = attr.Raw.Value
			break
		}
	}

	return &SMARTInfo{
		Health:             health,
		WearPercent:        s.NVMeSMARTHealthLog.PercentageUsed,
		HoursOn:            s.PowerOnTime.Hours,
		TemperatureCelsius: s.Temperature.Current,
		ReallocatedSectors: reallocated,
	}
}

// collectBlockStats reads /proc/diskstats for devName and returns block-layer I/O stats.
// devName may be a full path ("/dev/sda") or bare name ("sda").
// Returns nil if the device is not found (e.g. stub environments).
func (c *InventoryCollector) collectBlockStats(devName string) *BlockStats {
	// Strip "/dev/" prefix to obtain the bare kernel device name.
	bare := strings.TrimPrefix(devName, "/dev/")
	// Handle nested paths like "stub-sda" — just use as-is (won't match real diskstats).

	data, err := c.fs.ReadFile("/proc/diskstats")
	if err != nil {
		c.logger.Debug("cannot read /proc/diskstats", zap.Error(err))
		return nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		// /proc/diskstats has at least 14 fields per line.
		if len(fields) < 14 {
			continue
		}
		// Field 2 (0-based) is the device name.
		if fields[2] != bare {
			continue
		}

		readsCompleted, _ := strconv.ParseInt(fields[3], 10, 64)
		sectorsRead, _ := strconv.ParseInt(fields[5], 10, 64)
		writesCompleted, _ := strconv.ParseInt(fields[7], 10, 64)
		sectorsWritten, _ := strconv.ParseInt(fields[9], 10, 64)
		inFlight, _ := strconv.ParseInt(fields[11], 10, 64)

		return &BlockStats{
			ReadBytes:  sectorsRead * 512,
			WriteBytes: sectorsWritten * 512,
			ReadIops:   readsCompleted,
			WriteIops:  writesCompleted,
			InFlight:   inFlight,
		}
	}

	// Device not found in /proc/diskstats — normal for stub environments.
	return nil
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

func (c *InventoryCollector) stubSMART(_ *DeviceInfo) *SMARTInfo {
	return &SMARTInfo{
		Health:             "PASSED",
		WearPercent:        5,
		HoursOn:            500,
		TemperatureCelsius: 35,
		ReallocatedSectors: 0,
	}
}
