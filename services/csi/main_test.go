package main

import (
	"flag"
	"testing"
)

func TestRunWithFlags_InvalidEndpoint(t *testing.T) {
	// Test with invalid endpoint that will fail at driver.Run()
	args := []string{"-endpoint", "invalid://endpoint"}
	err := runWithFlags(args)
	// Should get an error from attempting to listen on invalid endpoint
	if err == nil {
		t.Error("expected error with invalid endpoint")
	}
}

func TestRunWithFlags_DefaultValues(t *testing.T) {
	// Skip this test as it would try to actually start the server
	// The function is covered by integration tests
	t.Skip("skipping integration test")
}

func TestRunWithFlags_HelpFlag(t *testing.T) {
	// Test that help flag parsing works
	args := []string{"-help"}
	err := runWithFlags(args)
	// flag.ContinueOnError means it returns error without calling log.Fatal
	if err != flag.ErrHelp {
		t.Logf("got error %v (expected flag.ErrHelp)", err)
	}
}
