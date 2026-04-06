package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c-mueller/ts-restic-server/internal/acl"
	"github.com/c-mueller/ts-restic-server/internal/config"
	"github.com/c-mueller/ts-restic-server/internal/middleware"
	"github.com/c-mueller/ts-restic-server/internal/storage/memory"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// setupACLServer creates a full Echo server with PlainIdentity middleware,
// ACL middleware, and API handlers backed by a memory backend.
func setupACLServer(t *testing.T, engine *acl.Engine, appendOnly bool) *echo.Echo {
	t.Helper()
	backend := memory.New(10 * 1024 * 1024)
	logger := zap.NewNop()

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	metricsCfg := config.MetricsConfig{Enabled: false}

	RegisterRoutes(e, backend, logger, appendOnly, engine, middleware.PlainIdentity(), false, metricsCfg, "", true)

	return e
}

func TestPlainMode_IPBasedACL_Allowed(t *testing.T) {
	engine, _ := acl.New(acl.Deny, []acl.Rule{
		{
			Paths:      []string{"/"},
			Identities: []string{"10.0.0.1"},
			Permission: acl.ReadOnly,
		},
	})

	srv := setupACLServer(t, engine, false)

	req := httptest.NewRequest(http.MethodGet, "/config", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	// Should get 404 (repo not found) rather than 403, proving ACL allowed the request
	if rec.Code == http.StatusForbidden {
		t.Fatalf("expected non-403 status (ACL should allow), got %d", rec.Code)
	}
}

func TestPlainMode_IPBasedACL_Denied(t *testing.T) {
	engine, _ := acl.New(acl.Deny, []acl.Rule{
		{
			Paths:      []string{"/"},
			Identities: []string{"10.0.0.1"},
			Permission: acl.ReadOnly,
		},
	})

	srv := setupACLServer(t, engine, false)

	// Different IP - should be denied
	req := httptest.NewRequest(http.MethodGet, "/config", nil)
	req.RemoteAddr = "10.0.0.99:12345"
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 (default deny), got %d", rec.Code)
	}
}

func TestPlainMode_DenialResponse_ContainsOnlyIP(t *testing.T) {
	engine, _ := acl.New(acl.Deny, []acl.Rule{})
	srv := setupACLServer(t, engine, false)

	req := httptest.NewRequest(http.MethodGet, "/config", nil)
	req.RemoteAddr = "192.168.1.50:12345"
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	// Top-level should contain error
	if body["error"] != "access denied" {
		t.Errorf("expected error='access denied', got %v", body["error"])
	}

	// Data field contains identity details
	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data field in response, got: %s", rec.Body.String())
	}

	if data["ip"] != "192.168.1.50" {
		t.Errorf("expected ip='192.168.1.50', got %v", data["ip"])
	}

	// Should NOT contain hostname, user, or tags (plain mode has none)
	for _, key := range []string{"hostname", "user", "tags"} {
		if _, exists := data[key]; exists {
			t.Errorf("plain mode denial should not contain %q in data, got: %v", key, data[key])
		}
	}
}

func TestPlainMode_ReadOnly_BlocksWrite(t *testing.T) {
	engine, _ := acl.New(acl.Deny, []acl.Rule{
		{
			Paths:      []string{"/"},
			Identities: []string{"*"},
			Permission: acl.ReadOnly,
		},
	})

	srv := setupACLServer(t, engine, false)

	// GET should be allowed
	req := httptest.NewRequest(http.MethodGet, "/config", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code == http.StatusForbidden {
		t.Fatal("GET should be allowed with read-only permission")
	}

	// POST should be denied (write requires append-only)
	req = httptest.NewRequest(http.MethodPost, "/config", strings.NewReader("test"))
	req.RemoteAddr = "10.0.0.1:12345"
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST should be denied with read-only permission, got %d", rec.Code)
	}
}

func TestPlainMode_AppendOnly_AllowsWriteBlocksDelete(t *testing.T) {
	engine, _ := acl.New(acl.Deny, []acl.Rule{
		{
			Paths:      []string{"/"},
			Identities: []string{"*"},
			Permission: acl.AppendOnly,
		},
	})

	srv := setupACLServer(t, engine, false)

	// POST should be allowed (write)
	req := httptest.NewRequest(http.MethodPost, "/config", strings.NewReader("test"))
	req.RemoteAddr = "10.0.0.1:12345"
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code == http.StatusForbidden {
		t.Fatal("POST should be allowed with append-only permission")
	}

	// DELETE on data blobs should be denied (requires full-access)
	req = httptest.NewRequest(http.MethodDelete, "/data/abc123", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("DELETE should be denied with append-only permission, got %d", rec.Code)
	}
}

func TestPlainMode_TrustedProxy_XForwardedFor(t *testing.T) {
	// Allow only 10.0.0.5 (the real client behind the proxy)
	engine, _ := acl.New(acl.Deny, []acl.Rule{
		{
			Paths:      []string{"/"},
			Identities: []string{"10.0.0.5"},
			Permission: acl.ReadOnly,
		},
	})

	backend := memory.New(10 * 1024 * 1024)
	logger := zap.NewNop()

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// Configure trusted proxy so X-Forwarded-For is honored
	e.IPExtractor = echo.ExtractIPFromXFFHeader(
		echo.TrustLoopback(false),
		echo.TrustLinkLocal(false),
		echo.TrustPrivateNet(true), // Trust private network proxies
	)

	metricsCfg := config.MetricsConfig{Enabled: false}
	RegisterRoutes(e, backend, logger, false, engine, middleware.PlainIdentity(), false, metricsCfg, "", true)

	// Request from proxy 192.168.1.1 with real client 10.0.0.5
	req := httptest.NewRequest(http.MethodGet, "/config", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	req.Header.Set("X-Forwarded-For", "10.0.0.5")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	// Should be allowed because XFF extracts 10.0.0.5 which matches the rule
	if rec.Code == http.StatusForbidden {
		t.Fatal("expected ACL to allow via X-Forwarded-For IP extraction")
	}
}

func TestPlainMode_PerRepoPathACL(t *testing.T) {
	engine, _ := acl.New(acl.Deny, []acl.Rule{
		{
			Paths:      []string{"/server-a"},
			Identities: []string{"10.0.0.1"},
			Permission: acl.FullAccess,
		},
		{
			Paths:      []string{"/server-b"},
			Identities: []string{"10.0.0.2"},
			Permission: acl.FullAccess,
		},
	})

	srv := setupACLServer(t, engine, false)

	// 10.0.0.1 accessing /server-a — allowed
	req := httptest.NewRequest(http.MethodGet, "/server-a/config", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code == http.StatusForbidden {
		t.Fatal("10.0.0.1 should be allowed on /server-a")
	}

	// 10.0.0.1 accessing /server-b — denied
	req = httptest.NewRequest(http.MethodGet, "/server-b/config", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("10.0.0.1 should be denied on /server-b, got %d", rec.Code)
	}

	// 10.0.0.2 accessing /server-b — allowed
	req = httptest.NewRequest(http.MethodGet, "/server-b/config", nil)
	req.RemoteAddr = "10.0.0.2:12345"
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code == http.StatusForbidden {
		t.Fatal("10.0.0.2 should be allowed on /server-b")
	}
}

func TestPlainMode_SystemRoutes_BypassACL(t *testing.T) {
	// Even with default deny, system routes should work
	engine, _ := acl.New(acl.Deny, []acl.Rule{})
	srv := setupACLServer(t, engine, false)

	// /-/ui/ should bypass ACL and not return 403
	req := httptest.NewRequest(http.MethodGet, "/-/ui/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code == http.StatusForbidden {
		t.Fatal("system route /-/ui/ should bypass ACL even with default deny")
	}
}
