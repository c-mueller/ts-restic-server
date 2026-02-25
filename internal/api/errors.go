package api

import (
	"github.com/c-mueller/ts-restic-server/internal/apierror"
	"github.com/c-mueller/ts-restic-server/internal/middleware"
	"github.com/labstack/echo/v4"
)

// apiError returns a standardized JSON error response.
func apiError(c echo.Context, status int, errStr string, message string) error {
	return apierror.New(c, status, errStr, message, middleware.GetRequestID(c.Request().Context()))
}

// apiErrorWithData returns a standardized JSON error response with additional data.
func apiErrorWithData(c echo.Context, status int, errStr string, message string, data interface{}) error {
	return apierror.WithData(c, status, errStr, message, middleware.GetRequestID(c.Request().Context()), data)
}
