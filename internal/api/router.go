package api

import (
	"github.com/chrismcg/ts-restic-server/internal/middleware"
	"github.com/chrismcg/ts-restic-server/internal/storage"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

func RegisterRoutes(e *echo.Echo, backend storage.Backend, logger *zap.Logger, appendOnly bool) {
	h := &Handler{
		Backend:    backend,
		Logger:     logger,
		AppendOnly: appendOnly,
	}

	// Pre-routing: strip repo path prefix before route matching
	e.Pre(middleware.RepoPrefix())

	e.Use(middleware.Recover(logger))
	e.Use(middleware.RequestID())
	e.Use(middleware.Logger(logger))

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
