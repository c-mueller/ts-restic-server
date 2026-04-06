package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/c-mueller/ts-restic-server/internal/acl"
	"github.com/c-mueller/ts-restic-server/internal/apierror"
	"github.com/c-mueller/ts-restic-server/internal/metrics"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// ACL returns middleware that checks requests against the ACL engine.
// If engine is nil, all requests are passed through (no-op).
// System routes (/-/) bypass ACL by default. When metricsACLEnabled is
// true, the /-/metrics endpoint is subject to ACL rules instead of its
// own Basic Auth. When verboseDenials is true, denial responses include
// identity details; when false, only a minimal error with request_id.
// Server-side logging always includes full identity regardless.
func ACL(engine *acl.Engine, logger *zap.Logger, verboseDenials, metricsACLEnabled bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		if engine == nil {
			return next
		}
		return func(c echo.Context) error {
			// System routes bypass ACL unless explicitly opted in.
			reqPath := c.Request().URL.Path
			if strings.HasPrefix(reqPath, "/-/") {
				if !(metricsACLEnabled && strings.HasPrefix(reqPath, "/-/metrics")) {
					return next(c)
				}
			}

			identities := GetIdentity(c.Request().Context())
			if len(identities) == 0 {
				identities = []string{c.RealIP()}
			}
			repoPrefix := GetRepoPrefix(c.Request().Context())
			repoPath := "/" + repoPrefix
			op := classifyOperation(c.Request().Method, c.Request().URL.Path)

			aclStart := time.Now()
			allowed := engine.Allowed(identities, repoPath, op)
			aclResult := "allowed"
			if !allowed {
				aclResult = "denied"
			}
			if metrics.Registry != nil {
				metrics.ACLEvaluationDuration.WithLabelValues(aclResult).Observe(time.Since(aclStart).Seconds())
				metrics.ACLDecisionsTotal.WithLabelValues(aclResult).Inc()
			}

			if !allowed {
				logger.Warn("acl denied",
					zap.String("request_id", GetRequestID(c.Request().Context())),
					zap.Strings("identities", identities),
					zap.String("repo_path", repoPath),
					zap.String("operation", opName(op)),
					zap.String("method", c.Request().Method),
					zap.String("path", c.Request().URL.Path),
				)
				return aclDeniedResponse(c, repoPath, op, verboseDenials)
			}
			return next(c)
		}
	}
}

// aclDeniedResponse constructs a standardized JSON error response for ACL denials.
// When verbose is false, only a minimal response with request_id is returned.
func aclDeniedResponse(c echo.Context, repoPath string, op acl.OperationType, verbose bool) error {
	if !verbose {
		return apierror.WithData(c, http.StatusForbidden, "access denied", "", GetRequestID(c.Request().Context()), nil)
	}

	data := map[string]interface{}{
		"path":      repoPath,
		"operation": opName(op),
		"ip":        c.RealIP(),
	}

	if whoIs := GetWhoIsResult(c.Request().Context()); whoIs != nil {
		if whoIs.FQDN != "" {
			data["hostname"] = whoIs.FQDN
		}
		if whoIs.LoginName != "" {
			data["user"] = whoIs.LoginName
		}
		if len(whoIs.Tags) > 0 {
			data["tags"] = whoIs.Tags
		}
	}

	return apierror.WithData(c, http.StatusForbidden, "access denied", "", GetRequestID(c.Request().Context()), data)
}

// classifyOperation maps an HTTP method and path to an ACL operation type.
func classifyOperation(method, path string) acl.OperationType {
	switch method {
	case http.MethodGet, http.MethodHead:
		return acl.OpRead
	case http.MethodDelete:
		// Lock deletion is a write-level operation, not a delete
		if strings.HasPrefix(path, "/locks/") || path == "/locks" {
			return acl.OpWrite
		}
		return acl.OpDelete
	default: // POST, PUT, PATCH
		return acl.OpWrite
	}
}

func opName(op acl.OperationType) string {
	switch op {
	case acl.OpRead:
		return "read"
	case acl.OpWrite:
		return "write"
	case acl.OpDelete:
		return "delete"
	default:
		return "unknown"
	}
}
