package api

import (
	"net/http"

	"github.com/c-mueller/ts-restic-server/internal/middleware"
	"github.com/c-mueller/ts-restic-server/internal/storage"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

func (h *Handler) ListBlobs(c echo.Context) error {
	ctx := c.Request().Context()
	reqID := middleware.GetRequestID(ctx)
	t := storage.BlobType(c.Param("type"))

	if !storage.ValidBlobTypes[t] {
		return c.NoContent(http.StatusBadRequest)
	}

	blobs, err := h.Backend.ListBlobs(ctx, t)
	if err != nil {
		h.Logger.Error("list blobs failed", zap.String("request_id", reqID), zap.Error(err))
		return c.NoContent(http.StatusInternalServerError)
	}
	if blobs == nil {
		blobs = []storage.Blob{}
	}

	if isV2(c) {
		c.Response().Header().Set("Content-Type", resticV2Type)
		return c.JSON(http.StatusOK, blobs)
	}

	// v1: return array of names
	names := make([]string, len(blobs))
	for i, b := range blobs {
		names[i] = b.Name
	}
	return c.JSON(http.StatusOK, names)
}
