package cmd

import (
	"context"
	"fmt"
	"net"
	"os/signal"
	"syscall"

	"github.com/c-mueller/ts-restic-server/internal/acl"
	"github.com/c-mueller/ts-restic-server/internal/config"
	"github.com/c-mueller/ts-restic-server/internal/server"
	"github.com/c-mueller/ts-restic-server/internal/storage"
	"github.com/c-mueller/ts-restic-server/internal/storage/filesystem"
	"github.com/c-mueller/ts-restic-server/internal/storage/memory"
	rclonebackend "github.com/c-mueller/ts-restic-server/internal/storage/rclone"
	s3backend "github.com/c-mueller/ts-restic-server/internal/storage/s3"
	webdavbackend "github.com/c-mueller/ts-restic-server/internal/storage/webdav"
	"github.com/labstack/echo/v4"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the Restic REST server",
	RunE:  runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.Flags().String("listen", "", "listen address (default :8000)")
	serveCmd.Flags().String("listen-mode", "", "listener mode: plain or tailscale")
	serveCmd.Flags().Bool("append-only", false, "enable append-only mode")
	serveCmd.Flags().String("log-level", "", "log level: debug, info, warn, error")
	serveCmd.Flags().String("storage-backend", "", "storage backend: filesystem, s3, memory")
	serveCmd.Flags().String("storage-path", "", "path for filesystem backend")

	viper.BindPFlag("listen", serveCmd.Flags().Lookup("listen"))
	viper.BindPFlag("listen_mode", serveCmd.Flags().Lookup("listen-mode"))
	viper.BindPFlag("append_only", serveCmd.Flags().Lookup("append-only"))
	viper.BindPFlag("log_level", serveCmd.Flags().Lookup("log-level"))
	viper.BindPFlag("storage.backend", serveCmd.Flags().Lookup("storage-backend"))
	viper.BindPFlag("storage.path", serveCmd.Flags().Lookup("storage-path"))
}

func runServe(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger, err := buildLogger(cfg.LogLevel)
	if err != nil {
		return err
	}
	defer logger.Sync()

	backend, err := buildBackend(cfg)
	if err != nil {
		return err
	}

	aclEngine, err := buildACLEngine(cfg.ACL)
	if err != nil {
		return err
	}

	ipExtractor, err := buildIPExtractor(cfg)
	if err != nil {
		return err
	}

	srv := server.New(cfg, backend, logger, aclEngine, ipExtractor)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return srv.Run(ctx)
}

func buildLogger(level string) (*zap.Logger, error) {
	zapCfg := zap.NewProductionConfig()

	switch level {
	case "debug":
		zapCfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		zapCfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		zapCfg.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		zapCfg.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	}

	return zapCfg.Build()
}

func buildACLEngine(cfg *config.ACLConfig) (*acl.Engine, error) {
	if cfg == nil {
		return nil, nil
	}

	defaultPerm, err := acl.ParsePermission(cfg.DefaultRole)
	if err != nil {
		return nil, err
	}

	rules := make([]acl.Rule, len(cfg.Rules))
	for i, r := range cfg.Rules {
		perm, err := acl.ParsePermission(r.Permission)
		if err != nil {
			return nil, err
		}
		rules[i] = acl.Rule{
			Paths:      r.Paths,
			Identities: r.Identities,
			Permission: perm,
		}
	}

	return acl.New(defaultPerm, rules)
}

func buildIPExtractor(cfg *config.Config) (echo.IPExtractor, error) {
	// Tailscale: direct connection, no proxy headers needed.
	if cfg.ListenMode == "tailscale" {
		return echo.ExtractIPDirect(), nil
	}

	// Plain mode without ACL: safe default, ignore proxy headers.
	if cfg.ACL == nil {
		return echo.ExtractIPDirect(), nil
	}

	// Plain mode with ACL: evaluate X-Forwarded-For from trusted proxies.
	if len(cfg.ACL.TrustedProxies) == 0 {
		// No explicit proxies — use Echo defaults (loopback + link-local + private nets).
		return echo.ExtractIPFromXFFHeader(), nil
	}

	// Explicit trusted proxies: trust only the configured CIDRs.
	opts := []echo.TrustOption{
		echo.TrustLoopback(false),
		echo.TrustLinkLocal(false),
		echo.TrustPrivateNet(false),
	}
	for _, cidr := range cfg.ACL.TrustedProxies {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("parsing trusted proxy CIDR %q: %w", cidr, err)
		}
		opts = append(opts, echo.TrustIPRange(ipNet))
	}
	return echo.ExtractIPFromXFFHeader(opts...), nil
}

func buildBackend(cfg *config.Config) (storage.Backend, error) {
	switch cfg.Storage.Backend {
	case "memory":
		return memory.New(cfg.Storage.MaxMemoryBytes), nil
	case "filesystem":
		return filesystem.New(cfg.Storage.Path, cfg.Storage.DataSharding)
	case "s3":
		return s3backend.New(context.Background(), cfg.Storage.S3.Bucket, cfg.Storage.S3.Prefix, cfg.Storage.S3.Region, cfg.Storage.S3.Endpoint, cfg.Storage.S3.AccessKey, cfg.Storage.S3.SecretKey)
	case "webdav":
		return webdavbackend.New(cfg.Storage.WebDAV.Endpoint, cfg.Storage.WebDAV.Username, cfg.Storage.WebDAV.Password, cfg.Storage.WebDAV.Prefix, cfg.Storage.DataSharding), nil
	case "rclone":
		return rclonebackend.New(cfg.Storage.Rclone.Endpoint, cfg.Storage.Rclone.Username, cfg.Storage.Rclone.Password), nil
	default:
		return nil, &config.ValidationError{Field: "storage.backend", Value: cfg.Storage.Backend}
	}
}
