package main

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"

	"go.uber.org/zap"
)

// mockCommandRunner is a test double for CommandRunner.
type mockCommandRunner struct {
	outputFn func(ctx context.Context, name string, args ...string) ([]byte, error)
	runFn    func(ctx context.Context, name string, args ...string) error
}

func (m *mockCommandRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	if m.outputFn != nil {
		return m.outputFn(ctx, name, args...)
	}
	return nil, exec.ErrNotFound
}

func (m *mockCommandRunner) Run(ctx context.Context, name string, args ...string) error {
	if m.runFn != nil {
		return m.runFn(ctx, name, args...)
	}
	return exec.ErrNotFound
}

// mockFileReader is a test double for FileReader.
type mockFileReader struct {
	readFileFn func(name string) ([]byte, error)
}

func (m *mockFileReader) ReadFile(name string) ([]byte, error) {
	if m.readFileFn != nil {
		return m.readFileFn(name)
	}
	return nil, errors.New("not found")
}

// newTestInventoryCollector builds a collector with injected mocks.
func newTestInventoryCollector(cmd CommandRunner, fs FileReader) *InventoryCollector {
	logger := zap.NewNop()
	ic := NewInventoryCollector("node-1", logger)
	if cmd != nil {
		ic.cmd = cmd
	}
	if fs != nil {
		ic.fs = fs
	}
	return ic
}

// ---- lsblk tests ----

func TestLsblk_Success(t *testing.T) {
	// Simulate lsblk output: name, serial, model, size, rota, type
	lsblkOutput := "sda  SERIAL001  WD-Ultrastar  18000000000000  1  disk\n" +
		"nvme0n1  NVMe001  Samsung980  2000000000000  0  disk\n" +
		"sda1  -  -  500000000000  1  part\n" // should be filtered
	cmd := &mockCommandRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name == "lsblk" {
				return []byte(lsblkOutput), nil
			}
			return nil, exec.ErrNotFound
		},
	}
	ic := newTestInventoryCollector(cmd, nil)

	devices, err := ic.lsblk(context.Background())
	if err != nil {
		t.Fatalf("lsblk error: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected 2 disk devices, got %d", len(devices))
	}
	// First device
	if devices[0].Name != "/dev/sda" {
		t.Errorf("device[0].Name = %s, want /dev/sda", devices[0].Name)
	}
	if devices[0].Serial != "SERIAL001" {
		t.Errorf("device[0].Serial = %s, want SERIAL001", devices[0].Serial)
	}
	// NVMe device
	if devices[1].Name != "/dev/nvme0n1" {
		t.Errorf("device[1].Name = %s, want /dev/nvme0n1", devices[1].Name)
	}
}

func TestLsblk_Error(t *testing.T) {
	cmd := &mockCommandRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			return nil, errors.New("lsblk not found")
		},
	}
	ic := newTestInventoryCollector(cmd, nil)
	_, err := ic.lsblk(context.Background())
	if err == nil {
		t.Error("expected error from lsblk failure")
	}
}

func TestLsblk_EmptyOutput(t *testing.T) {
	cmd := &mockCommandRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name == "lsblk" {
				return []byte(""), nil
			}
			return nil, exec.ErrNotFound
		},
	}
	ic := newTestInventoryCollector(cmd, nil)
	devices, err := ic.lsblk(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 0 {
		t.Errorf("expected 0 devices for empty output, got %d", len(devices))
	}
}

func TestLsblk_ShortLines(t *testing.T) {
	// Lines with fewer than 6 fields should be skipped
	output := "sda  SERIAL\n" // only 2 fields
	cmd := &mockCommandRunner{
		outputFn: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return []byte(output), nil
		},
	}
	ic := newTestInventoryCollector(cmd, nil)
	devices, err := ic.lsblk(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 0 {
		t.Errorf("expected 0 devices for short lines, got %d", len(devices))
	}
}

// ---- detectState tests ----

func TestDetectState_Active(t *testing.T) {
	cmd := &mockCommandRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name == "lsblk" {
				return []byte("/mnt/data\n"), nil
			}
			return nil, exec.ErrNotFound
		},
	}
	ic := newTestInventoryCollector(cmd, nil)
	d := &DeviceInfo{Name: "/dev/sda"}
	state := ic.detectState(d)
	if state != "Active" {
		t.Errorf("expected Active, got %s", state)
	}
}

func TestDetectState_DarkWhenNoMount(t *testing.T) {
	cmd := &mockCommandRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name == "lsblk" {
				return []byte("\n"), nil // empty mount point
			}
			return nil, exec.ErrNotFound
		},
	}
	ic := newTestInventoryCollector(cmd, nil)
	d := &DeviceInfo{Name: "/dev/sda"}
	state := ic.detectState(d)
	if state != "Dark" {
		t.Errorf("expected Dark when no mount, got %s", state)
	}
}

func TestDetectState_DarkWhenError(t *testing.T) {
	cmd := &mockCommandRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			return nil, errors.New("lsblk failed")
		},
	}
	ic := newTestInventoryCollector(cmd, nil)
	d := &DeviceInfo{Name: "/dev/sda"}
	state := ic.detectState(d)
	if state != "Dark" {
		t.Errorf("expected Dark when error, got %s", state)
	}
}

// ---- collectSMART tests ----

func TestCollectSMART_ValidJSON(t *testing.T) {
	smartJSON := `{
		"smart_status": {"passed": true},
		"temperature": {"current": 42},
		"power_on_time": {"hours": 1234},
		"ata_smart_attributes": {
			"table": [
				{"id": 5, "raw": {"value": 3}},
				{"id": 9, "raw": {"value": 100}}
			]
		},
		"nvme_smart_health_information_log": {"percentage_used": 15}
	}`
	cmd := &mockCommandRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name == "smartctl" {
				return []byte(smartJSON), nil
			}
			return nil, exec.ErrNotFound
		},
	}
	ic := newTestInventoryCollector(cmd, nil)
	result := ic.collectSMART(context.Background(), "/dev/sda")
	if result == nil {
		t.Fatal("expected non-nil SMARTInfo")
	}
	if result.Health != "PASSED" {
		t.Errorf("Health = %s, want PASSED", result.Health)
	}
	if result.TemperatureCelsius != 42 {
		t.Errorf("TemperatureCelsius = %d, want 42", result.TemperatureCelsius)
	}
	if result.HoursOn != 1234 {
		t.Errorf("HoursOn = %d, want 1234", result.HoursOn)
	}
	if result.WearPercent != 15 {
		t.Errorf("WearPercent = %d, want 15", result.WearPercent)
	}
	if result.ReallocatedSectors != 3 {
		t.Errorf("ReallocatedSectors = %d, want 3", result.ReallocatedSectors)
	}
}

func TestCollectSMART_FailedHealthMock(t *testing.T) {
	smartJSON := `{"smart_status": {"passed": false}, "temperature": {}, "power_on_time": {}, "ata_smart_attributes": {"table": []}, "nvme_smart_health_information_log": {}}`
	cmd := &mockCommandRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name == "smartctl" {
				return []byte(smartJSON), nil
			}
			return nil, exec.ErrNotFound
		},
	}
	ic := newTestInventoryCollector(cmd, nil)
	result := ic.collectSMART(context.Background(), "/dev/sda")
	if result == nil {
		t.Fatal("expected non-nil SMARTInfo")
	}
	if result.Health != "FAILED" {
		t.Errorf("Health = %s, want FAILED", result.Health)
	}
}

func TestCollectSMART_BadJSON(t *testing.T) {
	cmd := &mockCommandRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name == "smartctl" {
				return []byte("not json at all {{{"), nil
			}
			return nil, exec.ErrNotFound
		},
	}
	ic := newTestInventoryCollector(cmd, nil)
	result := ic.collectSMART(context.Background(), "/dev/sda")
	// Should fallback to stub
	if result == nil {
		t.Error("expected stub fallback, not nil")
	}
}

func TestCollectSMART_CommandError(t *testing.T) {
	cmd := &mockCommandRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			return nil, errors.New("smartctl failed")
		},
	}
	ic := newTestInventoryCollector(cmd, nil)
	result := ic.collectSMART(context.Background(), "/dev/sda")
	// Should fallback to stub
	if result == nil {
		t.Error("expected stub fallback, not nil")
	}
}

func TestCollectSMART_NoReallocAttr(t *testing.T) {
	// Table without attr ID 5 — reallocated should be 0
	smartJSON := `{
		"smart_status": {"passed": true},
		"temperature": {"current": 30},
		"power_on_time": {"hours": 100},
		"ata_smart_attributes": {"table": [{"id": 9, "raw": {"value": 50}}]},
		"nvme_smart_health_information_log": {"percentage_used": 0}
	}`
	cmd := &mockCommandRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name == "smartctl" {
				return []byte(smartJSON), nil
			}
			return nil, exec.ErrNotFound
		},
	}
	ic := newTestInventoryCollector(cmd, nil)
	result := ic.collectSMART(context.Background(), "/dev/sda")
	if result.ReallocatedSectors != 0 {
		t.Errorf("ReallocatedSectors = %d, want 0", result.ReallocatedSectors)
	}
}

func TestCollectSMART_DevicePathNormalization(t *testing.T) {
	// Device with nested path should be normalized to bare name
	var capturedArgs []string
	cmd := &mockCommandRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name == "smartctl" {
				capturedArgs = args
				return []byte(`{"smart_status":{"passed":true},"temperature":{},"power_on_time":{},"ata_smart_attributes":{},"nvme_smart_health_information_log":{}}`), nil
			}
			return nil, exec.ErrNotFound
		},
	}
	ic := newTestInventoryCollector(cmd, nil)
	ic.collectSMART(context.Background(), "/dev/mapper/sda")
	if len(capturedArgs) == 0 {
		t.Fatal("expected smartctl args to be captured")
	}
	// Last arg should be the device path — verify it starts with /dev/
	lastArg := capturedArgs[len(capturedArgs)-1]
	if len(lastArg) == 0 || lastArg[0] != '/' {
		t.Errorf("expected device path to start with /, got %q", lastArg)
	}
}

// ---- collectBlockStats tests ----

func TestCollectBlockStats_Found(t *testing.T) {
	// Realistic /proc/diskstats line
	diskstats := "   8   0 sda 1000 200 4000 5000 500 100 2000 3000 0 1500 8000 0 0 0 0\n"
	fs := &mockFileReader{
		readFileFn: func(_ string) ([]byte, error) {
			return []byte(diskstats), nil
		},
	}
	ic := newTestInventoryCollector(nil, fs)
	bs := ic.collectBlockStats("/dev/sda")
	if bs == nil {
		t.Fatal("expected non-nil BlockStats")
	}
	if bs.ReadIops != 1000 {
		t.Errorf("ReadIops = %d, want 1000", bs.ReadIops)
	}
	if bs.ReadBytes != 4000*512 {
		t.Errorf("ReadBytes = %d, want %d", bs.ReadBytes, 4000*512)
	}
	if bs.WriteIops != 500 {
		t.Errorf("WriteIops = %d, want 500", bs.WriteIops)
	}
	if bs.WriteBytes != 2000*512 {
		t.Errorf("WriteBytes = %d, want %d", bs.WriteBytes, 2000*512)
	}
	if bs.InFlight != 0 {
		t.Errorf("InFlight = %d, want 0", bs.InFlight)
	}
}

func TestCollectBlockStats_DeviceNotFound(t *testing.T) {
	diskstats := "   8   0 sdb 100 0 400 500 50 0 200 300 0 150 800 0 0 0 0\n"
	fs := &mockFileReader{
		readFileFn: func(_ string) ([]byte, error) {
			return []byte(diskstats), nil
		},
	}
	ic := newTestInventoryCollector(nil, fs)
	bs := ic.collectBlockStats("/dev/sda") // sda not in diskstats
	if bs != nil {
		t.Error("expected nil when device not found in diskstats")
	}
}

func TestCollectBlockStats_ReadError(t *testing.T) {
	fs := &mockFileReader{
		readFileFn: func(_ string) ([]byte, error) {
			return nil, errors.New("cannot read /proc/diskstats")
		},
	}
	ic := newTestInventoryCollector(nil, fs)
	bs := ic.collectBlockStats("/dev/sda")
	if bs != nil {
		t.Error("expected nil when file read fails")
	}
}

func TestCollectBlockStats_ShortLines(t *testing.T) {
	// Lines with fewer than 14 fields are skipped
	diskstats := "   8   0 sda 1000 200 4000\n" // only 7 fields
	fs := &mockFileReader{
		readFileFn: func(_ string) ([]byte, error) {
			return []byte(diskstats), nil
		},
	}
	ic := newTestInventoryCollector(nil, fs)
	bs := ic.collectBlockStats("/dev/sda")
	if bs != nil {
		t.Error("expected nil for short diskstats line")
	}
}

func TestCollectBlockStats_MultipleDevices(t *testing.T) {
	diskstats := "   8   0 sda 100 0 400 500 50 0 200 300 1 150 800 0 0 0 0\n" +
		"   8  16 sdb 999 0 3996 4995 499 0 1996 2994 5 1494 7992 0 0 0 0\n"
	fs := &mockFileReader{
		readFileFn: func(_ string) ([]byte, error) {
			return []byte(diskstats), nil
		},
	}
	ic := newTestInventoryCollector(nil, fs)

	// Request sdb specifically
	bs := ic.collectBlockStats("sdb") // bare name without /dev/
	if bs == nil {
		t.Fatal("expected non-nil BlockStats for sdb")
	}
	if bs.ReadIops != 999 {
		t.Errorf("ReadIops = %d, want 999", bs.ReadIops)
	}
	if bs.InFlight != 5 {
		t.Errorf("InFlight = %d, want 5", bs.InFlight)
	}
}

// ---- Collect integration tests ----

func TestCollect_FullPath(t *testing.T) {
	// Simulate a working lsblk + detect + smart + blockstats chain
	lsblkOut := "sda  SERIAL-COLLECT  WD-Ultrastar  18000000000000  1  disk\n"
	smartJSON := `{"smart_status":{"passed":true},"temperature":{"current":35},"power_on_time":{"hours":500},"ata_smart_attributes":{"table":[]},"nvme_smart_health_information_log":{"percentage_used":5}}`
	diskstats := "   8   0 sda 100 0 400 500 50 0 200 300 0 150 800 0 0 0 0\n"

	cmd := &mockCommandRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			switch name {
			case "lsblk":
				// Both lsblk calls: listing and mountpoint check
				// Distinguish by args
				for _, a := range args {
					if a == "MOUNTPOINT" || a == "-no" {
						return []byte(""), nil // no mount → Dark
					}
				}
				return []byte(lsblkOut), nil
			case "smartctl":
				return []byte(smartJSON), nil
			}
			return nil, exec.ErrNotFound
		},
	}
	fs := &mockFileReader{
		readFileFn: func(_ string) ([]byte, error) {
			return []byte(diskstats), nil
		},
	}
	ic := newTestInventoryCollector(cmd, fs)

	devices, err := ic.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect error: %v", err)
	}
	if len(devices) == 0 {
		t.Fatal("expected at least one device")
	}
	d := devices[0]
	if d.SMART == nil {
		t.Error("expected SMART data")
	}
	if d.BlockStats == nil {
		t.Error("expected BlockStats data")
	}
	if d.Class == "" {
		t.Error("expected non-empty class")
	}
}

// ---- collectSMARTWithScheduling caching tests ----

func TestCollectSMARTWithScheduling_CacheHit(t *testing.T) {
	callCount := 0
	smartJSON := `{"smart_status":{"passed":true},"temperature":{"current":30},"power_on_time":{"hours":200},"ata_smart_attributes":{"table":[]},"nvme_smart_health_information_log":{"percentage_used":3}}`
	cmd := &mockCommandRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name == "smartctl" {
				callCount++
				return []byte(smartJSON), nil
			}
			return nil, exec.ErrNotFound
		},
	}
	ic := newTestInventoryCollector(cmd, nil)
	ctx := context.Background()

	// First call — no cache; should call smartctl
	r1 := ic.collectSMARTWithScheduling(ctx, "/dev/sda")
	if r1 == nil {
		t.Fatal("expected non-nil from first call")
	}
	if callCount != 1 {
		t.Errorf("expected 1 smartctl call, got %d", callCount)
	}

	// Second call — cache is fresh; should NOT call smartctl
	r2 := ic.collectSMARTWithScheduling(ctx, "/dev/sda")
	if r2 == nil {
		t.Fatal("expected non-nil from second call")
	}
	if callCount != 1 {
		t.Errorf("expected still 1 smartctl call after cache hit, got %d", callCount)
	}
}

func TestCollectSMARTWithScheduling_CacheExpiry(t *testing.T) {
	callCount := 0
	smartJSON := `{"smart_status":{"passed":true},"temperature":{},"power_on_time":{},"ata_smart_attributes":{"table":[]},"nvme_smart_health_information_log":{}}`
	cmd := &mockCommandRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name == "smartctl" {
				callCount++
				return []byte(smartJSON), nil
			}
			return nil, exec.ErrNotFound
		},
	}
	ic := newTestInventoryCollector(cmd, nil)
	ctx := context.Background()
	devName := "/dev/sdb"

	// Seed cache with an old timestamp
	ic.mu.Lock()
	ic.lastSMARTScan[devName] = time.Now().Add(-25 * time.Hour)
	ic.lastSMART[devName] = &SMARTInfo{Health: "PASSED"}
	ic.mu.Unlock()

	// Call with stale cache — should re-run smartctl
	result := ic.collectSMARTWithScheduling(ctx, devName)
	if result == nil {
		t.Fatal("expected non-nil after cache expiry")
	}
	if callCount != 1 {
		t.Errorf("expected 1 smartctl call after cache expiry, got %d", callCount)
	}
}
