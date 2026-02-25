package metrics

import (
	"crypto/subtle"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Handler returns an Echo handler that serves the Prometheus metrics endpoint.
// If password is non-empty, Basic Auth is required (username: "prometheus").
func Handler(password string) echo.HandlerFunc {
	h := promhttp.HandlerFor(Registry, promhttp.HandlerOpts{})

	return func(c echo.Context) error {
		if password != "" {
			user, pass, ok := c.Request().BasicAuth()
			if !ok ||
				subtle.ConstantTimeCompare([]byte(user), []byte("prometheus")) != 1 ||
				subtle.ConstantTimeCompare([]byte(pass), []byte(password)) != 1 {
				c.Response().Header().Set("WWW-Authenticate", `Basic realm="metrics"`)
				return c.NoContent(http.StatusUnauthorized)
			}
		}
		h.ServeHTTP(c.Response(), c.Request())
		return nil
	}
}
