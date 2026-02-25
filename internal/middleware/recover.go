package middleware

import (
	"net/http"

	"github.com/c-mueller/ts-restic-server/internal/apierror"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

func Recover(logger *zap.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			defer func() {
				if r := recover(); r != nil {
					reqID := GetRequestID(c.Request().Context())
					logger.Error("panic recovered",
						zap.String("request_id", reqID),
						zap.Any("panic", r),
					)
					apierror.New(c, http.StatusInternalServerError, "internal server error", "", reqID)
				}
			}()
			return next(c)
		}
	}
}
