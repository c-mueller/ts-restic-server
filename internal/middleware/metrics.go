package middleware

import (
	"strconv"
	"time"

	"github.com/c-mueller/ts-restic-server/internal/metrics"
	"github.com/labstack/echo/v4"
)

// Metrics returns middleware that records HTTP request metrics.
// It should be positioned after ACL in the middleware chain so identity context is available.
func Metrics() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()

			err := next(c)

			duration := time.Since(start).Seconds()
			status := c.Response().Status
			statusStr := strconv.Itoa(status)
			method := c.Request().Method
			path := c.Path() // Echo route pattern, not actual URL — bounded cardinality

			metrics.HTTPRequestDuration.WithLabelValues(method, path, statusStr).Observe(duration)
			metrics.HTTPRequestsTotal.WithLabelValues(method, path, statusStr).Inc()

			if status >= 400 {
				metrics.HTTPErrorsTotal.WithLabelValues(method, path, statusStr).Inc()
			}

			// Per-host metrics (only when enabled to avoid unbounded cardinality)
			if metrics.PerHostEnabled {
				identities := GetIdentity(c.Request().Context())
				identity := c.RealIP()
				if len(identities) > 0 {
					identity = identities[0]
				}
				repoPath := "/" + GetRepoPrefix(c.Request().Context())

				metrics.HostRequestsTotal.WithLabelValues(identity, repoPath, method).Inc()

				if c.Request().ContentLength > 0 {
					metrics.HostBytesReceivedTotal.WithLabelValues(identity, repoPath).Add(float64(c.Request().ContentLength))
				}
				metrics.HostBytesSentTotal.WithLabelValues(identity, repoPath).Add(float64(c.Response().Size))
			}

			return err
		}
	}
}
