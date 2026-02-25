package middleware

import (
	"net/http"
	"strings"

	"github.com/c-mueller/ts-restic-server/internal/acl"
	"github.com/c-mueller/ts-restic-server/internal/apierror"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// ACL returns middleware that checks requests against the ACL engine.
// If engine is nil, all requests are passed through (no-op).
func ACL(engine *acl.Engine, logger *zap.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		if engine == nil {
			return next
		}
		return func(c echo.Context) error {
			identities := GetIdentity(c.Request().Context())
			if identities == nil {
				identities = []string{c.RealIP()}
			}
			repoPrefix := GetRepoPrefix(c.Request().Context())
			repoPath := "/" + repoPrefix
			op := classifyOperation(c.Request().Method, c.Request().URL.Path)

			if !engine.Allowed(identities, repoPath, op) {
				logger.Warn("acl denied",
					zap.String("request_id", GetRequestID(c.Request().Context())),
					zap.Strings("identities", identities),
					zap.String("repo_path", repoPath),
					zap.String("operation", opName(op)),
					zap.String("method", c.Request().Method),
					zap.String("path", c.Request().URL.Path),
				)
				return aclDeniedResponse(c, repoPath, op)
			}
			return next(c)
		}
	}
}

// aclDeniedResponse constructs a standardized JSON error response for ACL denials.
func aclDeniedResponse(c echo.Context, repoPath string, op acl.OperationType) error {
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
