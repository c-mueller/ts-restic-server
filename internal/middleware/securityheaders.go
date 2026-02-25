package middleware

import "github.com/labstack/echo/v4"

// SecurityHeaders returns middleware that sets security-related HTTP response headers.
// When tlsEnabled is true (e.g., Tailscale mode), the Strict-Transport-Security
// header is also set to enforce HTTPS.
func SecurityHeaders(tlsEnabled bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			h := c.Response().Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Cache-Control", "no-store")
			if tlsEnabled {
				h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
			}
			return next(c)
		}
	}
}
