package clitest

import (
	"os"
	"testing"
)

// TestMain scopes every addch metadata temp file created by this suite to a
// private directory. The package both calls media.WriteTempMetadata directly
// and spawns real addch child binaries that do the same; without isolation
// those "addch-metadata-*" files land in the shared system temp directory and
// race the cmd/addch suite, whose countTempMetadata asserts an absolute count
// of that pattern, when the two packages run concurrently under `go test ./...`.
func TestMain(m *testing.M) {
	isolated, err := os.MkdirTemp("", "clitest-metadata-tmp-")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("ADDCH_METADATA_TMPDIR", isolated)
	code := m.Run()
	_ = os.RemoveAll(isolated)
	os.Exit(code)
}
