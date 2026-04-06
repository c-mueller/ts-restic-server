package integration

import (
	"context"
	"testing"
	"time"

	smbbackend "github.com/c-mueller/ts-restic-server/internal/storage/smb"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestSMBBackend(t *testing.T) {
	t.Parallel()
	requireDocker(t)

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

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("get container host: %v", err)
	}
	mappedPort, err := container.MappedPort(ctx, "445")
	if err != nil {
		t.Fatalf("get mapped port: %v", err)
	}

	// Give Samba a moment to fully initialize after the port is open.
	time.Sleep(2 * time.Second)

	backend, err := smbbackend.New(host, mappedPort.Int(), "testshare", "testuser", "testpass", "WORKGROUP", "restic-integration")
	if err != nil {
		t.Fatalf("create smb backend: %v", err)
	}
	t.Cleanup(func() { backend.Close() })

	runBackendSuite(t, backend)
}
