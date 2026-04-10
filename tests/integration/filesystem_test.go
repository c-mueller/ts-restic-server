package integration

import (
	"testing"

	"github.com/c-mueller/ts-restic-server/internal/storage/filesystem"
)

func TestFilesystemBackend(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	backend, err := filesystem.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	runBackendSuite(t, backend)
}
