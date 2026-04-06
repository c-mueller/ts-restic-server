package smb_test

import (
	"context"
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/c-mueller/ts-restic-server/internal/storage"
	"github.com/c-mueller/ts-restic-server/internal/storage/backendtest"
	smbbackend "github.com/c-mueller/ts-restic-server/internal/storage/smb"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func requireDocker(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker not available, skipping SMB test")
	}
}

// startSMBContainer starts a Samba container and returns the host, mapped port,
// and cleanup function. The share "testshare" is writable by user "testuser"
// with password "testpass".
func startSMBContainer(t *testing.T) (host string, port int) {
	t.Helper()

	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "dperson/samba",
			ExposedPorts: []string{"445/tcp"},
			Cmd: []string{
				"-u", "testuser;testpass", // pragma: allowlist secret
				"-s", "testshare;/share;no;no;no;testuser",
				"-p",
			},
			WaitingFor: wait.ForListeningPort("445/tcp").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start samba container: %v", err)
	}
	t.Cleanup(func() { container.Terminate(ctx) })

	host, err = container.Host(ctx)
	if err != nil {
		t.Fatalf("get container host: %v", err)
	}
	mappedPort, err := container.MappedPort(ctx, "445")
	if err != nil {
		t.Fatalf("get mapped port: %v", err)
	}

	// Give Samba a moment to fully initialize after the port is open.
	time.Sleep(2 * time.Second)

	return host, mappedPort.Int()
}

func TestSuite(t *testing.T) {
	requireDocker(t)
	if testing.Short() {
		t.Skip("skipping SMB test in short mode")
	}

	host, port := startSMBContainer(t)

	// Each sub-test gets a unique base_path so they don't collide.
	counter := 0
	backendtest.RunSuite(t, func(t *testing.T) storage.Backend {
		counter++
		basePath := fmt.Sprintf("test-%d", counter)
		b, err := smbbackend.New(host, port, "testshare", "testuser", "testpass", "WORKGROUP", basePath)
		if err != nil {
			t.Fatalf("create smb backend: %v", err)
		}
		t.Cleanup(func() { b.Close() })
		return b
	})
}
