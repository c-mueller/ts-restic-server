package nfs_test

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/c-mueller/ts-restic-server/internal/storage"
	"github.com/c-mueller/ts-restic-server/internal/storage/backendtest"
	nfsbackend "github.com/c-mueller/ts-restic-server/internal/storage/nfs"
	"github.com/docker/docker/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func requireDocker(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker not available, skipping NFS test")
	}
}

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

// startNFSContainer starts an NFS server container with host networking.
// Host networking is required because the NFS client uses the portmapper
// (port 111) to discover mount and NFS service ports dynamically.
// The container exports /nfsshare with no_root_squash.
func startNFSContainer(t *testing.T) {
	t.Helper()

	// Pre-flight: check that required ports are available.
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
}

func TestSuite(t *testing.T) {
	requireDocker(t)
	requireLinux(t)
	if testing.Short() {
		t.Skip("skipping NFS test in short mode")
	}

	startNFSContainer(t)

	// Each sub-test gets a unique base_path so they don't collide.
	counter := 0
	backendtest.RunSuite(t, func(t *testing.T) storage.Backend {
		counter++
		basePath := fmt.Sprintf("test-%d", counter)
		b, err := nfsbackend.New("127.0.0.1", "/nfsshare", basePath, 0, 0)
		if err != nil {
			t.Skipf("could not connect to NFS server: %v", err)
		}
		t.Cleanup(func() { b.Close() })
		return b
	})
}
