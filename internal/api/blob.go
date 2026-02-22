package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/chrismcg/ts-restic-server/internal/middleware"
	"github.com/chrismcg/ts-restic-server/internal/storage"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

func (h *Handler) HeadBlob(c echo.Context) error {
	ctx := c.Request().Context()
	t, name := blobParams(c)

	if !storage.ValidBlobTypes[t] {
		return c.NoContent(http.StatusBadRequest)
	}

	size, err := h.Backend.StatBlob(ctx, t, name)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return c.NoContent(http.StatusNotFound)
		}
		return c.NoContent(http.StatusInternalServerError)
	}

	c.Response().Header().Set("Content-Length", formatInt64(size))
	return c.NoContent(http.StatusOK)
}

func (h *Handler) GetBlob(c echo.Context) error {
	ctx := c.Request().Context()
	reqID := middleware.GetRequestID(ctx)
	t, name := blobParams(c)

	if !storage.ValidBlobTypes[t] {
		return c.NoContent(http.StatusBadRequest)
	}

	offset, length, rangeRequested := parseRange(c)

	rc, err := h.Backend.GetBlob(ctx, t, name, offset, length)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return c.NoContent(http.StatusNotFound)
		}
		h.Logger.Error("get blob failed", zap.String("request_id", reqID), zap.Error(err))
		return c.NoContent(http.StatusInternalServerError)
	}
	defer rc.Close()

	status := http.StatusOK
	if rangeRequested {
		status = http.StatusPartialContent
	}
	return c.Stream(status, "application/octet-stream", rc)
}

func (h *Handler) SaveBlob(c echo.Context) error {
	ctx := c.Request().Context()
	reqID := middleware.GetRequestID(ctx)
	t, name := blobParams(c)

	if !storage.ValidBlobTypes[t] {
		return c.NoContent(http.StatusBadRequest)
	}

	defer c.Request().Body.Close()

	err := h.Backend.SaveBlob(ctx, t, name, c.Request().Body)
	if err != nil {
		if errors.Is(err, storage.ErrQuotaExceeded) {
			return c.String(http.StatusInsufficientStorage, "quota exceeded")
		}
		h.Logger.Error("save blob failed", zap.String("request_id", reqID), zap.Error(err))
		return c.NoContent(http.StatusInternalServerError)
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) DeleteBlob(c echo.Context) error {
	ctx := c.Request().Context()
	reqID := middleware.GetRequestID(ctx)
	t, name := blobParams(c)

	if !storage.ValidBlobTypes[t] {
		return c.NoContent(http.StatusBadRequest)
	}

	// Append-only: forbid deletion except for locks
	if h.AppendOnly && t != storage.BlobLocks {
		return c.NoContent(http.StatusForbidden)
	}

	err := h.Backend.DeleteBlob(ctx, t, name)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return c.NoContent(http.StatusNotFound)
		}
		h.Logger.Error("delete blob failed", zap.String("request_id", reqID), zap.Error(err))
		return c.NoContent(http.StatusInternalServerError)
	}

	return c.NoContent(http.StatusOK)
}

func blobParams(c echo.Context) (storage.BlobType, string) {
	return storage.BlobType(c.Param("type")), c.Param("name")
}

func parseRange(c echo.Context) (offset, length int64, rangeRequested bool) {
	rangeHeader := c.Request().Header.Get("Range")
	if rangeHeader == "" {
		return 0, 0, false
	}

	// Parse "bytes=start-end"
	if !strings.HasPrefix(rangeHeader, "bytes=") {
		return 0, 0, false
	}

	parts := strings.SplitN(rangeHeader[6:], "-", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}

	start, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, false
	}

	offset = start
	rangeRequested = true

	if parts[1] != "" {
		end, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return start, 0, true
		}
		length = end - start + 1
	}

	return offset, length, rangeRequested
}
