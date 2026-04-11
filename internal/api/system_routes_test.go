package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c-mueller/ts-restic-server/internal/config"
	"github.com/c-mueller/ts-restic-server/internal/storage/memory"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// setupRouterServer creates an Echo instance with full route registration
// (API routes only, no UI) to test routing behavior.
func setupRouterServer(t *testing.T) *echo.Echo {
	t.Helper()
	backend := memory.New(10 * 1024 * 1024)
	logger := zap.NewNop()

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	metricsCfg := config.MetricsConfig{Enabled: false}
	RegisterRoutes(e, backend, logger, false, nil, nil, false, metricsCfg, "", true)
	return e
}

// TestSystemRoutes_NotMatchedAsBlobType verifies that paths under /-/ return
// 404 instead of being caught by the /:type/:name blob routes.
// This was a bug where /-/ui matched /:type/:name with type="-", name="ui",
// returning 400 "invalid blob type" instead of 404.
func TestSystemRoutes_NotMatchedAsBlobType(t *testing.T) {
	srv := setupRouterServer(t)

	paths := []string{
		"/-/ui",
		"/-/ui/",
		"/-/ui/repos/",
		"/-/unknown",
		"/-/anything/here",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)

			if rec.Code == http.StatusBadRequest {
				t.Errorf("GET %s returned 400 (matched blob route), want 404", path)
			}
			if rec.Code != http.StatusNotFound {
				t.Errorf("GET %s returned %d, want 404", path, rec.Code)
			}
		})
	}
}

// TestSystemRoutes_BlobRoutesStillWork verifies that normal blob routes
// are not broken by the system route catch-all.
func TestSystemRoutes_BlobRoutesStillWork(t *testing.T) {
	srv := setupRouterServer(t)

	tests := []struct {
		method string
		path   string
		want   int // expected status (not 404, since the catch-all shouldn't interfere)
	}{
		{http.MethodHead, "/config", http.StatusNotFound},        // repo not created, but route matches
		{http.MethodGet, "/data/", http.StatusOK},                // list empty
		{http.MethodHead, "/data/aabbccdd", http.StatusNotFound}, // blob not found
	}

	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)

			if rec.Code != tc.want {
				t.Errorf("%s %s returned %d, want %d", tc.method, tc.path, rec.Code, tc.want)
			}
		})
	}
}
