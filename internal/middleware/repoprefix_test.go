package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c-mueller/ts-restic-server/internal/apierror"
	"github.com/labstack/echo/v4"
)

// setupRepoPrefixEcho creates an Echo instance with RepoPrefix middleware
// and a catch-all handler that records the rewritten path and repo prefix.
func setupRepoPrefixEcho() *echo.Echo {
	e := echo.New()
	e.Pre(RepoPrefix())

	handler := func(c echo.Context) error {
		prefix := GetRepoPrefix(c.Request().Context())
		return c.JSON(http.StatusOK, map[string]string{
			"prefix": prefix,
			"path":   c.Request().URL.Path,
		})
	}

	// Register routes matching the real router.
	e.POST("/", handler)
	e.DELETE("/", handler)
	e.HEAD("/config", handler)
	e.GET("/config", handler)
	e.POST("/config", handler)
	e.GET("/:type/", handler)
	e.HEAD("/:type/:name", handler)
	e.GET("/:type/:name", handler)
	e.POST("/:type/:name", handler)
	e.DELETE("/:type/:name", handler)

	return e
}

func TestRepoPrefix_TraversalDotDot(t *testing.T) {
	e := setupRepoPrefixEcho()

	tests := []struct {
		name string
		path string
	}{
		{"dotdot in prefix", "/../../etc/passwd/config"},
		{"dotdot only", "/../config"},
		{"dotdot mid-path", "/host/../etc/keys/"},
		{"dotdot at start", "/../data/abc"},
		{"dotdot deep", "/a/b/../../../etc/config"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for path %q, got %d", tt.path, rec.Code)
			}

			var resp apierror.ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to parse error response: %v", err)
			}
			if resp.Status != http.StatusBadRequest {
				t.Errorf("expected status 400 in body, got %d", resp.Status)
			}
			if resp.Error != "bad request" {
				t.Errorf("expected error 'bad request', got %q", resp.Error)
			}
			if resp.Message == "" {
				t.Error("expected non-empty message")
			}
		})
	}
}

func TestRepoPrefix_TraversalDot(t *testing.T) {
	e := setupRepoPrefixEcho()

	tests := []struct {
		name string
		path string
	}{
		{"single dot", "/./config"},
		{"dot in prefix", "/host/./config"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for path %q, got %d", tt.path, rec.Code)
			}

			var resp apierror.ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to parse error response: %v", err)
			}
			if resp.Status != http.StatusBadRequest {
				t.Errorf("expected status 400 in body, got %d", resp.Status)
			}
		})
	}
}

func TestRepoPrefix_NullByte(t *testing.T) {
	e := setupRepoPrefixEcho()

	// Go's net/url rejects raw null bytes in URLs, so we construct the
	// request manually with the path set directly to simulate what a
	// malicious client might send after server-level URL decoding.
	tests := []struct {
		name string
		path string
	}{
		{"null in prefix", "/host\x00evil/config"},
		{"null in segment", "/\x00/data/abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/placeholder", nil)
			req.URL.Path = tt.path // bypass URL parser validation
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for path with null byte, got %d", rec.Code)
			}

			var resp apierror.ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to parse error response: %v", err)
			}
			if resp.Status != http.StatusBadRequest {
				t.Errorf("expected status 400 in body, got %d", resp.Status)
			}
		})
	}
}

func TestRepoPrefix_LegitimateMultiLevelPaths(t *testing.T) {
	e := setupRepoPrefixEcho()

	tests := []struct {
		name           string
		path           string
		expectedPrefix string
		expectedPath   string
	}{
		{"simple repo + config", "/myrepo/config", "myrepo", "/config"},
		{"multi-level repo + config", "/host/backup/config", "host/backup", "/config"},
		{"repo + data blob", "/myrepo/data/abc123", "myrepo", "/data/abc123"},
		{"deep repo + keys list", "/org/team/host/keys/", "org/team/host", "/keys/"},
		{"root config", "/config", "", "/config"},
		{"root data", "/data/abc", "", "/data/abc"},
		{"repo create", "/myrepo", "myrepo", "/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method := http.MethodGet
			if tt.expectedPath == "/" {
				method = http.MethodPost
			}
			req := httptest.NewRequest(method, tt.path, nil)
			if tt.expectedPath == "/" {
				q := req.URL.Query()
				q.Set("create", "true")
				req.URL.RawQuery = q.Encode()
			}
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200 for path %q, got %d (body: %s)", tt.path, rec.Code, rec.Body.String())
			}

			var result map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
				t.Fatalf("failed to parse response: %v", err)
			}
			if result["prefix"] != tt.expectedPrefix {
				t.Errorf("expected prefix %q, got %q", tt.expectedPrefix, result["prefix"])
			}
			if result["path"] != tt.expectedPath {
				t.Errorf("expected path %q, got %q", tt.expectedPath, result["path"])
			}
		})
	}
}

func TestRepoPrefix_ErrorResponseSchema(t *testing.T) {
	e := setupRepoPrefixEcho()

	req := httptest.NewRequest(http.MethodGet, "/../config", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	// Verify the response follows the standardized error schema.
	var raw map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	// Required fields must be present.
	for _, key := range []string{"status", "error"} {
		if _, exists := raw[key]; !exists {
			t.Errorf("missing required key %q in error response", key)
		}
	}

	if raw["status"].(float64) != 400 {
		t.Errorf("expected status 400, got %v", raw["status"])
	}

	// Optional fields: message should be present, data should not.
	if _, exists := raw["message"]; !exists {
		t.Error("expected message field for traversal error")
	}
	if _, exists := raw["data"]; exists {
		t.Error("data field should not be present for traversal error")
	}
}
