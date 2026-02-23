package api

import (
	"errors"
	"net/http"

	"github.com/c-mueller/ts-restic-server/internal/middleware"
	"github.com/c-mueller/ts-restic-server/internal/storage"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

func (h *Handler) CreateRepo(c echo.Context) error {
	ctx := c.Request().Context()
	reqID := middleware.GetRequestID(ctx)

	if c.QueryParam("create") != "true" {
		return c.NoContent(http.StatusBadRequest)
	}

	err := h.Backend.CreateRepo(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrRepoExists) {
			return c.NoContent(http.StatusOK)
		}
		h.Logger.Error("create repo failed", zap.String("request_id", reqID), zap.Error(err))
		return c.NoContent(http.StatusInternalServerError)
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) DeleteRepo(c echo.Context) error {
	ctx := c.Request().Context()
	reqID := middleware.GetRequestID(ctx)

	if h.AppendOnly {
		return c.NoContent(http.StatusForbidden)
	}

	err := h.Backend.DeleteRepo(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrRepoNotFound) {
			return c.NoContent(http.StatusNotFound)
		}
		h.Logger.Error("delete repo failed", zap.String("request_id", reqID), zap.Error(err))
		return c.NoContent(http.StatusInternalServerError)
	}

	return c.NoContent(http.StatusOK)
}
