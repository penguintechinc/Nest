package main

import (
	"testing"

	"go.uber.org/zap"
)

func TestNewCRPublisher_OutOfCluster(t *testing.T) {
	logger := zap.NewNop()
	// Not running in-cluster — should return nil publisher, nil error
	pub, err := NewCRPublisher("test-node", logger)
	if err != nil {
		t.Fatalf("NewCRPublisher returned error: %v", err)
	}
	if pub != nil {
		t.Error("expected nil publisher when not in-cluster")
	}
}

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple-node", "simple-node"},
		{"Node_With_Underscores", "node-with-underscores"},
		{"node/with/slashes", "node-with-slashes"},
		{"UPPER-CASE", "upper-case"},
		{"/dev/sda", "sda"},
		{"node--double--dash", "node-double-dash"},
		{"-leading-dash", "leading-dash"},
		{"trailing-dash-", "trailing-dash"},
		// Long name truncation (68 chars → truncated to 63)
		{"abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnop", "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijk"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeName(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDevicesToUnstructured(t *testing.T) {
	devices := []*DeviceInfo{
		{
			Name:          "/dev/sda",
			Serial:        "SN-001",
			Model:         "WD",
			CapacityBytes: 1024,
			Class:         "sata-bulk",
			State:         "Active",
			SMART: &SMARTInfo{
				Health:             "PASSED",
				WearPercent:        5,
				HoursOn:            100,
				TemperatureCelsius: 30,
				ReallocatedSectors: 0,
			},
			Signature: "sig-abc",
		},
		{
			Name:          "/dev/sdb",
			Serial:        "SN-002",
			Model:         "Seagate",
			CapacityBytes: 2048,
			Class:         "sata-cold",
			State:         "Dark",
			SMART:         nil,
			Signature:     "",
		},
	}

	result := devicesToUnstructured(devices)
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}

	first, ok := result[0].(map[string]interface{})
	if !ok {
		t.Fatal("first entry is not map[string]interface{}")
	}
	if first["serial"] != "SN-001" {
		t.Errorf("expected serial SN-001, got %v", first["serial"])
	}
	if _, hasSMART := first["smart"]; !hasSMART {
		t.Error("expected smart key for device with SMART data")
	}
	if _, hasSig := first["signature"]; !hasSig {
		t.Error("expected signature key when Signature is set")
	}

	second, ok := result[1].(map[string]interface{})
	if !ok {
		t.Fatal("second entry is not map[string]interface{}")
	}
	if _, hasSMART := second["smart"]; hasSMART {
		t.Error("expected no smart key for device without SMART data")
	}
	if _, hasSig := second["signature"]; hasSig {
		t.Error("expected no signature key when Signature is empty")
	}
}

func TestBoolPtr(t *testing.T) {
	b := boolPtr(true)
	if b == nil {
		t.Fatal("boolPtr returned nil")
	}
	if !*b {
		t.Error("expected *b to be true")
	}
	b2 := boolPtr(false)
	if *b2 {
		t.Error("expected *b2 to be false")
	}
}

func TestSanitizeName_MaxLength(t *testing.T) {
	// 64-char name should be truncated to 63
	longName := "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijk" // 63 chars exactly
	got := sanitizeName(longName)
	if len(got) > 63 {
		t.Errorf("sanitizeName result exceeds 63 chars: len=%d", len(got))
	}
}
