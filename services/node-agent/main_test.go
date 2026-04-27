package main

import (
	"testing"
)

func TestMain(t *testing.T) {
	// Test that main can be parsed/compiled
	t.Run("imports are valid", func(t *testing.T) {
		// This test ensures the main package compiles and imports are valid
		if testing.Short() {
			t.Skip("skipping in short mode")
		}
	})
}

// TestOsCommandRunner_OutputErrNotFound verifies osCommandRunner returns an error
// for a binary that does not exist.
func TestOsCommandRunner_OutputErrNotFound(t *testing.T) {
	r := osCommandRunner{}
	// Use a binary name that definitely doesn't exist
	_, err := r.Output(t.Context(), "nonexistent-binary-xyz999")
	if err == nil {
		t.Error("expected error for nonexistent binary")
	}
}

// TestOsCommandRunner_RunErrNotFound verifies Run also errors for missing binary.
func TestOsCommandRunner_RunErrNotFound(t *testing.T) {
	r := osCommandRunner{}
	err := r.Run(t.Context(), "nonexistent-binary-xyz999")
	if err == nil {
		t.Error("expected error for nonexistent binary")
	}
}

// TestOsFileReader_ReadFileError verifies osFileReader errors on missing file.
func TestOsFileReader_ReadFileError(t *testing.T) {
	r := osFileReader{}
	_, err := r.ReadFile("/nonexistent/path/xyz999")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

// TestDynamicCRClient_Compile verifies that dynamicCRClient satisfies CRClient interface.
// This is a compile-time check; if it compiles, the test passes.
func TestDynamicCRClient_Compile(t *testing.T) {
	var _ CRClient = &dynamicCRClient{}
}
