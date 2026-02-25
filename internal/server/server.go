package server

import (
	"context"
	"net"

	"github.com/c-mueller/ts-restic-server/internal/acl"
	"github.com/c-mueller/ts-restic-server/internal/api"
	"github.com/c-mueller/ts-restic-server/internal/config"
	"github.com/c-mueller/ts-restic-server/internal/storage"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
	"tailscale.com/tsnet"
)

type Server struct {
	cfg      *config.Config
	backend  storage.Backend
	logger   *zap.Logger
	echo     *echo.Echo
	tsServer *tsnet.Server // nil in plain mode
}

func New(cfg *config.Config, backend storage.Backend, logger *zap.Logger, aclEngine *acl.Engine, ipExtractor echo.IPExtractor, identityMW echo.MiddlewareFunc, tsServer *tsnet.Server) *Server {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	if ipExtractor != nil {
		e.IPExtractor = ipExtractor
	}

	api.RegisterRoutes(e, backend, logger, cfg.AppendOnly, aclEngine, identityMW, cfg.Metrics)

	return &Server{
		cfg:      cfg,
		backend:  backend,
		logger:   logger,
		echo:     e,
		tsServer: tsServer,
	}
}

func (s *Server) Run(ctx context.Context) error {
	ln, cleanup, err := NewListener(ctx, s.cfg, s.logger, s.tsServer)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}

	s.logger.Info("server starting",
		zap.String("listen", s.cfg.Listen),
		zap.String("mode", s.cfg.ListenMode),
		zap.String("backend", s.cfg.Storage.Backend),
	)

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.echo.Server.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		s.logger.Info("shutting down server")
		return s.echo.Shutdown(context.Background())
	case err := <-errCh:
		return err
	}
}

func (s *Server) ServeOnListener(ln net.Listener) error {
	return s.echo.Server.Serve(ln)
}

func (s *Server) Echo() *echo.Echo {
	return s.echo
}
