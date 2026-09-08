package main

import (
	"os"
	"testing"
)

// TestMain re-executes the real addch CLI when ADDCH_TEST_BINARY=1. This lets a
// test spawn the actual binary (os.Executable()) as a genuine child process for
// signal-handling tests. The env var check happens before m.Run(), so the
// testing framework never parses the CLI arguments as test flags.
func TestMain(m *testing.M) {
	if os.Getenv("ADDCH_TEST_BINARY") == "1" {
		os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}
