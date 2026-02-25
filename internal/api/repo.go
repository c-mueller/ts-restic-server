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
		return apiError(c, http.StatusBadRequest, "bad request", "repository does not exist and ?create=true was not provided")
	}

	err := h.Backend.CreateRepo(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrRepoExists) {
			return c.NoContent(http.StatusOK)
		}
		h.Logger.Error("create repo failed", zap.String("request_id", reqID), zap.Error(err))
		return apiError(c, http.StatusInternalServerError, "internal server error", "")
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) DeleteRepo(c echo.Context) error {
	ctx := c.Request().Context()
	reqID := middleware.GetRequestID(ctx)

	if h.AppendOnly {
		return apiError(c, http.StatusForbidden, "forbidden", "append-only mode: repository deletion is not allowed")
	}

	err := h.Backend.DeleteRepo(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrRepoNotFound) {
			return apiError(c, http.StatusNotFound, "not found", "repository not found")
		}
		h.Logger.Error("delete repo failed", zap.String("request_id", reqID), zap.Error(err))
		return apiError(c, http.StatusInternalServerError, "internal server error", "")
	}

	return c.NoContent(http.StatusOK)
}
