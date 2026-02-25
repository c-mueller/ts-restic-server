package server

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/c-mueller/ts-restic-server/internal/config"
	"github.com/c-mueller/ts-restic-server/internal/storage/memory"
	"go.uber.org/zap"
)

func TestRun_GracefulShutdown(t *testing.T) {
	cfg := &config.Config{
		Listen:          "127.0.0.1:0",
		ListenMode:      "plain",
		ShutdownTimeout: 2,
		Storage:         config.Storage{Backend: "memory", MaxMemoryBytes: 1024},
		Metrics:         config.MetricsConfig{Enabled: false},
	}
	backend := memory.New(1024)
	logger := zap.NewNop()

	srv := New(cfg, backend, logger, nil, nil, nil, nil)

	// Create a listener on an ephemeral port so Run() can start.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ServeOnListener(ln)
	}()

	// Give the server a moment to start serving.
	time.Sleep(50 * time.Millisecond)

	// Trigger shutdown via echo.Shutdown with the configured timeout.
	timeout := time.Duration(cfg.ShutdownTimeout) * time.Second
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), timeout)
	defer shutdownCancel()

	done := make(chan struct{})
	go func() {
		if err := srv.Echo().Shutdown(shutdownCtx); err != nil {
			t.Errorf("shutdown error: %v", err)
		}
		close(done)
	}()

	select {
	case <-done:
		// Shutdown completed within timeout.
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not complete within 5s")
	}

	cancel()
	_ = ctx // avoid unused variable
}
