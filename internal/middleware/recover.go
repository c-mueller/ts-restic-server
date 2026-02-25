package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/c-mueller/ts-restic-server/internal/apierror"
	"github.com/c-mueller/ts-restic-server/internal/metrics"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

func Recover(logger *zap.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			defer func() {
				if r := recover(); r != nil {
					if metrics.Registry != nil {
						metrics.PanicsTotal.Inc()
					}
					reqID := GetRequestID(c.Request().Context())
					logger.Error("panic recovered",
						zap.String("request_id", reqID),
						zap.String("panic", fmt.Sprintf("%v", r)),
						zap.String("stack", string(debug.Stack())),
					)
					if !c.Response().Committed {
						apierror.New(c, http.StatusInternalServerError, "internal server error", "", reqID)
					} else {
						logger.Warn("response already committed during panic recovery",
							zap.String("request_id", reqID),
						)
					}
				}
			}()
			return next(c)
		}
	}
}
