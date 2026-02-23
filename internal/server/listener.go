package server

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/c-mueller/ts-restic-server/internal/config"
	"go.uber.org/zap"
	"tailscale.com/tsnet"
)

func NewListener(ctx context.Context, cfg *config.Config, logger *zap.Logger) (net.Listener, func(), error) {
	switch cfg.ListenMode {
	case "plain":
		return newPlainListener(cfg.Listen)
	case "tailscale":
		return newTailscaleListener(ctx, cfg, logger)
	default:
		return nil, nil, fmt.Errorf("unknown listen_mode: %s", cfg.ListenMode)
	}
}

func newPlainListener(addr string) (net.Listener, func(), error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, nil, fmt.Errorf("listen on %s: %w", addr, err)
	}
	return ln, nil, nil
}

func newTailscaleListener(ctx context.Context, cfg *config.Config, logger *zap.Logger) (net.Listener, func(), error) {
	if err := os.MkdirAll(cfg.Tailscale.StateDir, 0o700); err != nil {
		return nil, nil, fmt.Errorf("create tailscale state directory %s: %w", cfg.Tailscale.StateDir, err)
	}

	srv := &tsnet.Server{
		Hostname: cfg.Tailscale.Hostname,
		Dir:      cfg.Tailscale.StateDir,
		AuthKey:  cfg.Tailscale.AuthKey,
	}

	logger.Info("starting tailscale node",
		zap.String("hostname", cfg.Tailscale.Hostname),
		zap.String("state_dir", cfg.Tailscale.StateDir),
	)

	ln, err := srv.ListenTLS("tcp", ":443")
	if err != nil {
		srv.Close()
		return nil, nil, fmt.Errorf("tailscale listen: %w", err)
	}

	cleanup := func() {
		ln.Close()
		srv.Close()
	}

	return ln, cleanup, nil
}
