package integration

import (
	"testing"

	"github.com/c-mueller/ts-restic-server/internal/storage/memory"
	rclonebackend "github.com/c-mueller/ts-restic-server/internal/storage/rclone"
)

func TestRcloneBackend(t *testing.T) {
	t.Parallel()
	// Start a "remote" restic REST server backed by memory storage.
	// The rclone backend proxies all operations to this server over HTTP,
	// exercising the full HTTP client implementation without needing rclone.
	remoteBackend := memory.New(512 * 1024 * 1024)
	remoteURL := startServer(t, remoteBackend)

	backend := rclonebackend.New(remoteURL, "", "")
	runBackendSuite(t, backend)
}
