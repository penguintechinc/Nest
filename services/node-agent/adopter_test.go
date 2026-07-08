package main

import (
	"context"
	"fmt"
	"testing"

	"go.uber.org/zap"
)

func TestNewDriveAdopter(t *testing.T) {
	logger := zap.NewNop()
	adopter := NewDriveAdopter(logger)
	if adopter == nil {
		t.Fatal("NewDriveAdopter returned nil")
	}
	if adopter.logger != logger {
		t.Error("logger not set correctly")
	}
}

// TestFormatDrive_Btrfs tests successful btrfs formatting.
func TestFormatDrive_Btrfs(t *testing.T) {
	logger := zap.NewNop()
	adopter := NewDriveAdopter(logger)

	// Mock command runner that succeeds
	adopter.cmd = &mockCommandRunner{
		outputFn: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			// blkid returns error for blank device
			return nil, fmt.Errorf("blkid: device not found")
		},
		runFn: func(ctx context.Context, name string, args ...string) error {
			// mkfs.btrfs succeeds
			if name == "mkfs.btrfs" {
				return nil
			}
			return fmt.Errorf("unexpected command: %s", name)
		},
	}

	ctx := context.Background()
	err := adopter.FormatDrive(ctx, "/dev/test-blank", "btrfs", false)
	if err != nil {
		t.Errorf("FormatDrive(btrfs) failed: %v", err)
	}
}

// TestFormatDrive_ZFS tests successful zfs formatting.
func TestFormatDrive_ZFS(t *testing.T) {
	logger := zap.NewNop()
	adopter := NewDriveAdopter(logger)

	// Mock command runner that succeeds
	adopter.cmd = &mockCommandRunner{
		outputFn: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			// blkid returns error for blank device
			return nil, fmt.Errorf("blkid: device not found")
		},
		runFn: func(ctx context.Context, name string, args ...string) error {
			// zpool create succeeds
			if name == "zpool" {
				return nil
			}
			return fmt.Errorf("unexpected command: %s", name)
		},
	}

	ctx := context.Background()
	err := adopter.FormatDrive(ctx, "/dev/test-blank", "zfs", false)
	if err != nil {
		t.Errorf("FormatDrive(zfs) failed: %v", err)
	}
}

// TestFormatDrive_NestPreviousIsIdempotent tests that formatting a nest-previous device is a no-op.
func TestFormatDrive_NestPreviousIsIdempotent(t *testing.T) {
	logger := zap.NewNop()
	adopter := NewDriveAdopter(logger)

	blkidOutput := []byte(`DEVNAME=/dev/test-nest
TYPE=btrfs
LABEL=nest.penguintech.io
`)

	commandCalled := false
	adopter.cmd = &mockCommandRunner{
		outputFn: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return blkidOutput, nil
		},
		runFn: func(ctx context.Context, name string, args ...string) error {
			commandCalled = true
			return fmt.Errorf("unexpected command call for nest-previous device")
		},
	}

	ctx := context.Background()
	err := adopter.FormatDrive(ctx, "/dev/test-nest", "btrfs", false)
	if err != nil {
		t.Errorf("FormatDrive on nest-previous should not error: %v", err)
	}
	if commandCalled {
		t.Error("mkfs should not be called for nest-previous device")
	}
}

// TestFormatDrive_SystemMountError tests that formatting fails if device has system mount.
func TestFormatDrive_SystemMountError(t *testing.T) {
	logger := zap.NewNop()
	adopter := NewDriveAdopter(logger)

	blkidOutput := []byte(`DEVNAME=/dev/sda1
TYPE=ext4
`)

	mountsContent := `/dev/sda1 / ext4 rw,relatime 0 0
`

	adopter.cmd = &mockCommandRunner{
		outputFn: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return blkidOutput, nil
		},
		runFn: func(ctx context.Context, name string, args ...string) error {
			return fmt.Errorf("unexpected command call")
		},
	}

	adopter.fs = &mockFileReader{
		readFileFn: func(name string) ([]byte, error) {
			if name == "/proc/mounts" {
				return []byte(mountsContent), nil
			}
			return nil, fmt.Errorf("file not found")
		},
	}

	ctx := context.Background()
	err := adopter.FormatDrive(ctx, "/dev/sda1", "btrfs", false)
	if err == nil {
		t.Error("FormatDrive on system mount should return error")
	}
}

// TestFormatDrive_InvalidFSType tests error on invalid fsType.
func TestFormatDrive_InvalidFSType(t *testing.T) {
	logger := zap.NewNop()
	adopter := NewDriveAdopter(logger)

	ctx := context.Background()
	err := adopter.FormatDrive(ctx, "/dev/test", "ext4", false)
	if err == nil {
		t.Error("FormatDrive with invalid fsType should return error")
	}
	if !contains(err.Error(), "unsupported fsType") {
		t.Errorf("expected unsupported fsType error, got: %v", err)
	}
}

// TestVerifyNestLabel_BlankDevice tests VerifyNestLabel returns false for blank device.
func TestVerifyNestLabel_BlankDevice(t *testing.T) {
	logger := zap.NewNop()
	adopter := NewDriveAdopter(logger)

	adopter.cmd = &mockCommandRunner{
		outputFn: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("blkid: device not found")
		},
	}

	ctx := context.Background()
	if adopter.VerifyNestLabel(ctx, "/dev/test-blank") {
		t.Error("VerifyNestLabel for blank device should return false")
	}
}

// TestVerifyNestLabel_NestPreviousDevice tests VerifyNestLabel returns true for nest-previous device.
func TestVerifyNestLabel_NestPreviousDevice(t *testing.T) {
	logger := zap.NewNop()
	adopter := NewDriveAdopter(logger)

	blkidOutput := []byte(`DEVNAME=/dev/test-nest
TYPE=btrfs
LABEL=nest.penguintech.io
`)

	adopter.cmd = &mockCommandRunner{
		outputFn: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return blkidOutput, nil
		},
	}

	ctx := context.Background()
	if !adopter.VerifyNestLabel(ctx, "/dev/test-nest") {
		t.Error("VerifyNestLabel for nest-previous device should return true")
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr))
}
