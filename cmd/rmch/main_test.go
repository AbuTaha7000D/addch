package main

import (
	"os"
	"testing"
)

// TestMain re-executes the real rmch CLI when RMCH_TEST_BINARY=1. This lets a
// test spawn the actual binary (os.Executable()) as a genuine child process for
// signal-handling tests. The env var check happens before m.Run(), so the
// testing framework never parses the CLI arguments as test flags.
//
// The test process runs with a private TMPDIR. rmch's temp metadata files are
// created under os.TempDir() (see WriteTempMetadata); isolating them here keeps
// rmch's temp-litter assertions scoped to rmch's own artifacts and prevents the
// addch suite (which globs the same addch-metadata-* pattern in the shared
// tempdir) from racing with rmch when both packages run concurrently.
func TestMain(m *testing.M) {
	if os.Getenv("RMCH_TEST_BINARY") == "1" {
		os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
	}
	isolated, err := os.MkdirTemp("", "rmch-tmp-")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("TMPDIR", isolated)
	code := m.Run()
	_ = os.RemoveAll(isolated)
	os.Exit(code)
}
