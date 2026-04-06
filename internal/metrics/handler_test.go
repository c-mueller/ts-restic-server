package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func init() {
	// Initialize the registry so Handler can serve metrics.
	Init("memory", true)
}

func TestHandler_NoPassword(t *testing.T) {
	e := echo.New()
	e.GET("/-/metrics", Handler(""))

	req := httptest.NewRequest(http.MethodGet, "/-/metrics", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "restic_server_build_info") {
		t.Error("response should contain restic_server_build_info metric")
	}
}

func TestHandler_CorrectPassword(t *testing.T) {
	e := echo.New()
	e.GET("/-/metrics", Handler("secret123")) // pragma: allowlist secret

	req := httptest.NewRequest(http.MethodGet, "/-/metrics", nil)
	req.SetBasicAuth("prometheus", "secret123") // pragma: allowlist secret
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "restic_server_build_info") {
		t.Error("response should contain restic_server_build_info metric")
	}
}

func TestHandler_WrongPassword(t *testing.T) {
	e := echo.New()
	e.GET("/-/metrics", Handler("correct-pw")) // pragma: allowlist secret

	req := httptest.NewRequest(http.MethodGet, "/-/metrics", nil)
	req.SetBasicAuth("prometheus", "wrong-pw") // pragma: allowlist secret
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	if got := rec.Header().Get("WWW-Authenticate"); got != `Basic realm="metrics"` {
		t.Errorf("WWW-Authenticate = %q, want %q", got, `Basic realm="metrics"`)
	}
}

func TestHandler_WrongUsername(t *testing.T) {
	e := echo.New()
	e.GET("/-/metrics", Handler("secret123")) // pragma: allowlist secret

	req := httptest.NewRequest(http.MethodGet, "/-/metrics", nil)
	req.SetBasicAuth("admin", "secret123") // pragma: allowlist secret
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandler_NoAuth_WhenPasswordRequired(t *testing.T) {
	e := echo.New()
	e.GET("/-/metrics", Handler("secret123")) // pragma: allowlist secret

	req := httptest.NewRequest(http.MethodGet, "/-/metrics", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
