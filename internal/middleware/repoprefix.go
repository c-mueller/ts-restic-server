package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/c-mueller/ts-restic-server/internal/apierror"
	"github.com/labstack/echo/v5"
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
//
// The path is checked and split by SplitRepoPath before any further processing.
func RepoPrefix() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			path := c.Request().URL.Path

			// System routes (e.g. /-/metrics) bypass repo prefix extraction.
			if strings.HasPrefix(path, "/-/") {
				return next(c)
			}

			segments, err := SplitRepoPath(path)
			if err != nil {
				return apierror.New(c, http.StatusBadRequest, "bad request",
					err.Error(), GetRequestID(c.Request().Context()))
			}

			for i, seg := range segments {
				if knownSegments[seg] {
					prefix := strings.Join(segments[:i], "/")
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
			prefix := strings.Join(segments, "/")
			ctx := context.WithValue(c.Request().Context(), repoPrefixKey{}, prefix)
			c.SetRequest(c.Request().WithContext(ctx))
			c.Request().URL.Path = "/"
			return next(c)
		}
	}
}

// SplitRepoPath splits a URL path into its segments. Empty segments are
// dropped: the backends join paths with path.Join or filepath.Join, which
// read "a//b" as "a/b", so the ACL has to see the same path. ".", "..", NUL
// bytes and backslashes (a separator on SMB) are rejected.
func SplitRepoPath(p string) ([]string, error) {
	var segments []string
	for _, seg := range strings.Split(p, "/") {
		switch {
		case seg == "":
			continue
		case seg == "." || seg == "..":
			return nil, errors.New("path traversal sequences are not allowed")
		case strings.ContainsAny(seg, "\x00\\"):
			return nil, errors.New("path contains invalid characters")
		}
		segments = append(segments, seg)
	}
	return segments, nil
}

// ContextWithRepoPrefix returns a new context with the given repo prefix set.
func ContextWithRepoPrefix(ctx context.Context, prefix string) context.Context {
	return context.WithValue(ctx, repoPrefixKey{}, prefix)
}

// GetRepoPrefix returns the repository path prefix from the context.
func GetRepoPrefix(ctx context.Context) string {
	if v, ok := ctx.Value(repoPrefixKey{}).(string); ok {
		return v
	}
	return ""
}
