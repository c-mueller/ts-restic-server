package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestSecurityHeaders_PlainMode(t *testing.T) {
	e := echo.New()
	e.Use(SecurityHeaders(false))
	e.GET("/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	tests := []struct {
		header, want string
	}{
		{"X-Content-Type-Options", "nosniff"},
		{"X-Frame-Options", "DENY"},
		{"Cache-Control", "no-store"},
	}
	for _, tc := range tests {
		if got := rec.Header().Get(tc.header); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.header, got, tc.want)
		}
	}

	// HSTS should NOT be present in plain mode
	if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("Strict-Transport-Security should be empty in plain mode, got %q", got)
	}
}

func TestSecurityHeaders_TailscaleMode(t *testing.T) {
	e := echo.New()
	e.Use(SecurityHeaders(true))
	e.GET("/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	expected := "max-age=63072000; includeSubDomains"
	if got := rec.Header().Get("Strict-Transport-Security"); got != expected {
		t.Errorf("Strict-Transport-Security = %q, want %q", got, expected)
	}
}

func TestSecurityHeaders_OnErrorResponses(t *testing.T) {
	e := echo.New()
	e.Use(SecurityHeaders(false))
	e.GET("/fail", func(c echo.Context) error {
		return c.String(http.StatusNotFound, "not found")
	})

	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	// Headers should be present even on error responses
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q on error response, want \"nosniff\"", got)
	}
}
