package middleware

import (
	"time"

	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

func Logger(logger *zap.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			start := time.Now()
			err := next(c)
			// The error handler runs after the chain returns, so resolve the
			// status the returned error will produce.
			_, status := echo.ResolveResponseStatus(c.Response(), err)

			reqID := GetRequestID(c.Request().Context())
			fields := []zap.Field{
				zap.String("request_id", reqID),
				zap.String("method", c.Request().Method),
				zap.String("path", c.Request().URL.Path),
				zap.String("query", c.Request().URL.RawQuery),
				zap.String("ip", c.RealIP()),
				zap.Int("status", status),
				zap.Duration("duration", time.Since(start)),
			}
			if ids := GetIdentity(c.Request().Context()); len(ids) > 1 {
				fields = append(fields, zap.Strings("identities", ids))
			}
			logger.Info("request", fields...)

			return err
		}
	}
}
