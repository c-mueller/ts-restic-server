package middleware

import (
	"net/http"

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
					c.NoContent(http.StatusInternalServerError)
				}
			}()
			return next(c)
		}
	}
}
