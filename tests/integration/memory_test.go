package integration

import (
	"testing"

	"github.com/c-mueller/ts-restic-server/internal/storage/memory"
)

func TestMemoryBackend(t *testing.T) {
	t.Parallel()
	runBackendSuite(t, memory.New(512*1024*1024))
}
