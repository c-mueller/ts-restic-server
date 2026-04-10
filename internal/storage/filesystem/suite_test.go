package filesystem_test

import (
	"testing"

	"github.com/c-mueller/ts-restic-server/internal/storage"
	"github.com/c-mueller/ts-restic-server/internal/storage/backendtest"
	"github.com/c-mueller/ts-restic-server/internal/storage/filesystem"
)

func TestSuite(t *testing.T) {
	backendtest.RunSuite(t, func(t *testing.T) storage.Backend {
		b, err := filesystem.New(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return b
	})
}
