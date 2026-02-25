package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c-mueller/ts-restic-server/internal/acl"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// newTestACLEngine creates an ACL engine that allows only the given identity on "/"
// with append-only permission (default deny).
func newTestACLEngine(allowedIdentity string) *acl.Engine {
	engine, err := acl.New(acl.Deny, []acl.Rule{
		{
			Paths:      []string{"/"},
			Identities: []string{allowedIdentity},
			Permission: acl.AppendOnly,
		},
	})
	if err != nil {
		panic(err)
	}
	return engine
}

// setupACLTest creates an Echo context with the given identities set in the context.
// If setIdentities is false, no identity is stored (simulating missing identity middleware).
func setupACLTest(setIdentities bool, identities []string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/config", nil)
	// Set RemoteAddr so RealIP() returns a known value
	req.RemoteAddr = "10.0.0.1:12345"

	if setIdentities {
		ctx := context.WithValue(req.Context(), identityKey{}, identities)
		req = req.WithContext(ctx)
	}

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return c, rec
}

func TestACLMiddleware_NilIdentity_FallsBackToRealIP(t *testing.T) {
	// Identity middleware not called → GetIdentity returns nil
	// ACL should fall back to RealIP ("10.0.0.1")
	engine := newTestACLEngine("10.0.0.1")
	logger := zap.NewNop()

	mw := ACL(engine, logger)
	c, rec := setupACLTest(false, nil)

	called := false
	handler := mw(func(c echo.Context) error {
		called = true
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("handler was not called; expected ACL to allow via RealIP fallback")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestACLMiddleware_EmptyIdentitySlice_FallsBackToRealIP(t *testing.T) {
	// Identity middleware returns []string{} → GetIdentity returns empty slice
	// ACL should fall back to RealIP ("10.0.0.1")
	engine := newTestACLEngine("10.0.0.1")
	logger := zap.NewNop()

	mw := ACL(engine, logger)
	c, rec := setupACLTest(true, []string{})

	called := false
	handler := mw(func(c echo.Context) error {
		called = true
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("handler was not called; expected ACL to allow via RealIP fallback for empty slice")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestACLMiddleware_WithIdentity_UsesProvidedIdentity(t *testing.T) {
	// Identity middleware returns a real identity
	// ACL should use the provided identity, not fall back
	engine := newTestACLEngine("host.example.com")
	logger := zap.NewNop()

	mw := ACL(engine, logger)
	c, rec := setupACLTest(true, []string{"host.example.com"})

	called := false
	handler := mw(func(c echo.Context) error {
		called = true
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("handler was not called; expected ACL to allow via provided identity")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestACLMiddleware_WithIdentity_DeniesUnknownIdentity(t *testing.T) {
	// Identity middleware returns an identity not in the ACL rules
	// ACL should deny access
	engine := newTestACLEngine("allowed.example.com")
	logger := zap.NewNop()

	mw := ACL(engine, logger)
	c, rec := setupACLTest(true, []string{"unknown.example.com"})

	called := false
	handler := mw(func(c echo.Context) error {
		called = true
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Fatal("handler was called; expected ACL to deny unknown identity")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rec.Code)
	}
}

func TestACLMiddleware_NilEngine_PassesThrough(t *testing.T) {
	// nil engine → no-op middleware, all requests pass through
	logger := zap.NewNop()

	mw := ACL(nil, logger)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/config", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	called := false
	handler := mw(func(c echo.Context) error {
		called = true
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("handler was not called; nil engine should pass through")
	}
}
