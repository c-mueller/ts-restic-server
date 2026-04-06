package api

import (
	"github.com/c-mueller/ts-restic-server/internal/acl"
	"github.com/c-mueller/ts-restic-server/internal/config"
	"github.com/c-mueller/ts-restic-server/internal/metrics"
	"github.com/c-mueller/ts-restic-server/internal/middleware"
	"github.com/c-mueller/ts-restic-server/internal/storage"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"
)

func RegisterRoutes(e *echo.Echo, backend storage.Backend, logger *zap.Logger, appendOnly bool, aclEngine *acl.Engine, identityMW echo.MiddlewareFunc, tlsEnabled bool, metricsCfg config.MetricsConfig, maxRequestBodySize string, verboseDenials bool) {
	h := &Handler{
		Backend:    backend,
		Logger:     logger,
		AppendOnly: appendOnly,
	}

	// System routes — registered directly on root, outside the API middleware chain.
	if metricsCfg.Enabled && metrics.Registry != nil {
		e.GET("/-/metrics", metrics.Handler(metricsCfg.Password))
	}

	// Pre-routing: strip repo path prefix before route matching
	e.Pre(middleware.RepoPrefix())

	// API middleware chain
	e.Use(middleware.Recover(logger))
	if maxRequestBodySize != "" {
		e.Use(echo_middleware.BodyLimit(maxRequestBodySize))
	}
	e.Use(middleware.SecurityHeaders(tlsEnabled))
	e.Use(middleware.RequestID())
	e.Use(middleware.Logger(logger))
	if identityMW != nil {
		e.Use(identityMW)
	}
	e.Use(middleware.ACL(aclEngine, logger, verboseDenials, metricsCfg.ACLEnabled))
	if metricsCfg.Enabled && metrics.Registry != nil {
		e.Use(middleware.Metrics())
	}

	// Repo management
	e.POST("/", h.CreateRepo)
	e.DELETE("/", h.DeleteRepo)

	// Config
	e.HEAD("/config", h.HeadConfig)
	e.GET("/config", h.GetConfig)
	e.POST("/config", h.SaveConfig)

	// Blob operations
	e.GET("/:type/", h.ListBlobs)
	e.HEAD("/:type/:name", h.HeadBlob)
	e.GET("/:type/:name", h.GetBlob)
	e.POST("/:type/:name", h.SaveBlob)
	e.DELETE("/:type/:name", h.DeleteBlob)
}
