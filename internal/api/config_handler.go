package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/chrismcg/ts-restic-server/internal/middleware"
	"github.com/chrismcg/ts-restic-server/internal/storage"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

func (h *Handler) HeadConfig(c echo.Context) error {
	ctx := c.Request().Context()

	size, err := h.Backend.StatConfig(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return c.NoContent(http.StatusNotFound)
		}
		return c.NoContent(http.StatusInternalServerError)
	}

	c.Response().Header().Set("Content-Length", formatInt64(size))
	return c.NoContent(http.StatusOK)
}

func (h *Handler) GetConfig(c echo.Context) error {
	ctx := c.Request().Context()
	reqID := middleware.GetRequestID(ctx)

	rc, err := h.Backend.GetConfig(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return c.NoContent(http.StatusNotFound)
		}
		h.Logger.Error("get config failed", zap.String("request_id", reqID), zap.Error(err))
		return c.NoContent(http.StatusInternalServerError)
	}
	defer rc.Close()

	return c.Stream(http.StatusOK, "application/octet-stream", rc)
}

func (h *Handler) SaveConfig(c echo.Context) error {
	ctx := c.Request().Context()
	reqID := middleware.GetRequestID(ctx)

	defer c.Request().Body.Close()

	err := h.Backend.SaveConfig(ctx, c.Request().Body)
	if err != nil {
		if errors.Is(err, storage.ErrQuotaExceeded) {
			return c.String(http.StatusInsufficientStorage, "quota exceeded")
		}
		h.Logger.Error("save config failed", zap.String("request_id", reqID), zap.Error(err))
		return c.NoContent(http.StatusInternalServerError)
	}

	return c.NoContent(http.StatusOK)
}

func formatInt64(n int64) string {
	return strconv.FormatInt(n, 10)
}
