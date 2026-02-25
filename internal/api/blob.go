package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/c-mueller/ts-restic-server/internal/middleware"
	"github.com/c-mueller/ts-restic-server/internal/storage"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

func (h *Handler) HeadBlob(c echo.Context) error {
	ctx := c.Request().Context()
	t, name := blobParams(c)

	if !storage.ValidBlobTypes[t] {
		return apiError(c, http.StatusBadRequest, "invalid blob type", fmt.Sprintf("unknown type %q", string(t)))
	}

	size, err := h.Backend.StatBlob(ctx, t, name)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return c.NoContent(http.StatusNotFound)
		}
		return c.NoContent(http.StatusInternalServerError)
	}

	c.Response().Header().Set("Content-Length", formatInt64(size))
	c.Response().Header().Set("Accept-Ranges", "bytes")
	return c.NoContent(http.StatusOK)
}

func (h *Handler) GetBlob(c echo.Context) error {
	ctx := c.Request().Context()
	reqID := middleware.GetRequestID(ctx)
	t, name := blobParams(c)

	if !storage.ValidBlobTypes[t] {
		return apiError(c, http.StatusBadRequest, "invalid blob type", fmt.Sprintf("unknown type %q", string(t)))
	}

	offset, length, rangeRequested := parseRange(c)

	if rangeRequested {
		// Get total size for range validation and Content-Range header
		totalSize, err := h.Backend.StatBlob(ctx, t, name)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return apiError(c, http.StatusNotFound, "not found", "")
			}
			h.Logger.Error("stat blob failed", zap.String("request_id", reqID), zap.Error(err))
			return apiError(c, http.StatusInternalServerError, "internal server error", "")
		}

		// Validate range start
		if offset >= totalSize {
			c.Response().Header().Set("Content-Range", fmt.Sprintf("bytes */%d", totalSize))
			return apiError(c, http.StatusRequestedRangeNotSatisfiable, "range not satisfiable", "")
		}

		// Clamp length to available data
		if length <= 0 || offset+length > totalSize {
			length = totalSize - offset
		}

		rc, err := h.Backend.GetBlob(ctx, t, name, offset, length)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return apiError(c, http.StatusNotFound, "not found", "")
			}
			h.Logger.Error("get blob failed", zap.String("request_id", reqID), zap.Error(err))
			return apiError(c, http.StatusInternalServerError, "internal server error", "")
		}
		defer rc.Close()

		end := offset + length - 1
		c.Response().Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, end, totalSize))
		c.Response().Header().Set("Content-Length", formatInt64(length))
		c.Response().Header().Set("Accept-Ranges", "bytes")
		return c.Stream(http.StatusPartialContent, "application/octet-stream", rc)
	}

	rc, err := h.Backend.GetBlob(ctx, t, name, 0, 0)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return apiError(c, http.StatusNotFound, "not found", "")
		}
		h.Logger.Error("get blob failed", zap.String("request_id", reqID), zap.Error(err))
		return apiError(c, http.StatusInternalServerError, "internal server error", "")
	}
	defer rc.Close()

	return c.Stream(http.StatusOK, "application/octet-stream", rc)
}

func (h *Handler) SaveBlob(c echo.Context) error {
	ctx := c.Request().Context()
	reqID := middleware.GetRequestID(ctx)
	t, name := blobParams(c)

	if !storage.ValidBlobTypes[t] {
		return apiError(c, http.StatusBadRequest, "invalid blob type", fmt.Sprintf("unknown type %q", string(t)))
	}

	defer c.Request().Body.Close()

	err := h.Backend.SaveBlob(ctx, t, name, c.Request().Body)
	if err != nil {
		if errors.Is(err, storage.ErrQuotaExceeded) {
			return apiError(c, http.StatusInsufficientStorage, "quota exceeded", "storage quota has been exceeded")
		}
		h.Logger.Error("save blob failed", zap.String("request_id", reqID), zap.Error(err))
		return apiError(c, http.StatusInternalServerError, "internal server error", "")
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) DeleteBlob(c echo.Context) error {
	ctx := c.Request().Context()
	reqID := middleware.GetRequestID(ctx)
	t, name := blobParams(c)

	if !storage.ValidBlobTypes[t] {
		return apiError(c, http.StatusBadRequest, "invalid blob type", fmt.Sprintf("unknown type %q", string(t)))
	}

	// Append-only: forbid deletion except for locks
	if h.AppendOnly && t != storage.BlobLocks {
		return apiError(c, http.StatusForbidden, "forbidden", "append-only mode: deletion is not allowed")
	}

	err := h.Backend.DeleteBlob(ctx, t, name)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return apiError(c, http.StatusNotFound, "not found", "")
		}
		h.Logger.Error("delete blob failed", zap.String("request_id", reqID), zap.Error(err))
		return apiError(c, http.StatusInternalServerError, "internal server error", "")
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
