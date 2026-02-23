package middleware

import (
	"context"
	"strings"

	"github.com/labstack/echo/v4"
)

type repoPrefixKey struct{}

var knownSegments = map[string]bool{
	"config":    true,
	"data":      true,
	"keys":      true,
	"locks":     true,
	"snapshots": true,
	"index":     true,
}

// RepoPrefix is a pre-routing middleware that extracts the repository path
// prefix from the URL. The prefix is stored in the request context so backends
// can use it to scope storage (e.g. different repos in the same S3 bucket).
// The URL is rewritten to strip the prefix so Echo's routes match correctly.
func RepoPrefix() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Request().URL.Path
			segments := strings.Split(strings.Trim(path, "/"), "/")

			for i, seg := range segments {
				if knownSegments[seg] {
					prefix := ""
					if i > 0 {
						prefix = strings.Join(segments[:i], "/")
					}
					apiPath := "/" + strings.Join(segments[i:], "/")
					if strings.HasSuffix(path, "/") {
						apiPath += "/"
					}
					ctx := context.WithValue(c.Request().Context(), repoPrefixKey{}, prefix)
					c.SetRequest(c.Request().WithContext(ctx))
					c.Request().URL.Path = apiPath
					return next(c)
				}
			}

			// No known API segment found — root-level operation (create/delete repo)
			prefix := strings.Trim(path, "/")
			ctx := context.WithValue(c.Request().Context(), repoPrefixKey{}, prefix)
			c.SetRequest(c.Request().WithContext(ctx))
			c.Request().URL.Path = "/"
			return next(c)
		}
	}
}

// GetRepoPrefix returns the repository path prefix from the context.
func GetRepoPrefix(ctx context.Context) string {
	if v, ok := ctx.Value(repoPrefixKey{}).(string); ok {
		return v
	}
	return ""
}
