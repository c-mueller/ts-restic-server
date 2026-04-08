package integration

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"testing"
	"time"

	nfsbackend "github.com/c-mueller/ts-restic-server/internal/storage/nfs"
	"github.com/docker/docker/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func requireLinux(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("NFS container tests require Linux (host networking); skipping on " + runtime.GOOS)
	}
}

func requirePortFree(t *testing.T, port int) {
	t.Helper()
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		t.Skipf("port %d is in use, skipping NFS test", port)
	}
	ln.Close()
}

func TestNFSBackend(t *testing.T) {
	t.Parallel()
	requireDocker(t)
	requireLinux(t)

	requirePortFree(t, 111)
	requirePortFree(t, 2049)

	ctx := context.Background()

	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "erichough/nfs-server",
			Env: map[string]string{
				"NFS_EXPORT_0": "/nfsshare *(rw,sync,no_subtree_check,no_root_squash)",
			},
			Privileged: true,
			HostConfigModifier: func(hc *container.HostConfig) {
				hc.NetworkMode = "host"
			},
			WaitingFor: wait.ForListeningPort("2049/tcp").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Skipf("could not start NFS container: %v", err)
	}
	t.Cleanup(func() { ctr.Terminate(ctx) })

	// Give NFS services time to register with the portmapper.
	time.Sleep(3 * time.Second)

	backend, err := nfsbackend.New("127.0.0.1", "/nfsshare", "restic-integration", 0, 0)
	if err != nil {
		t.Skipf("could not connect to NFS server: %v", err)
	}
	t.Cleanup(func() { backend.Close() })

	runBackendSuite(t, backend)
}
