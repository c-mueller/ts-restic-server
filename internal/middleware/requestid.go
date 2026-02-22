package middleware

import (
	"context"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type contextKey string

const RequestIDKey contextKey = "request_id"

func RequestID() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			id := uuid.New().String()
			c.Response().Header().Set("X-Request-ID", id)
			ctx := context.WithValue(c.Request().Context(), RequestIDKey, id)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}

func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(RequestIDKey).(string); ok {
		return id
	}
	return ""
}
