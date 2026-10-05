package ui

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c-mueller/ts-restic-server/internal/stats"
	"github.com/labstack/echo/v4"
)

const statsDisabledHint = "stats.enabled"

func serveUI(t *testing.T, store *stats.Store, path string) string {
	t.Helper()
	e := echo.New()
	if err := RegisterRoutes(e, store, nil, "", ""); err != nil {
		t.Fatalf("RegisterRoutes: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want %d", path, rec.Code, http.StatusOK)
	}
	return rec.Body.String()
}

func TestPages_WithoutStatsStore_ShowDisabledHint(t *testing.T) {
	for _, path := range []string{"/-/ui/", "/-/ui/repos/", "/-/ui/repos/host/backup/"} {
		body := serveUI(t, nil, path)
		if !strings.Contains(body, statsDisabledHint) {
			t.Errorf("GET %s: page should tell that stats are disabled (missing %q)", path, statsDisabledHint)
		}
		if strings.Contains(body, "No repository statistics recorded yet") {
			t.Errorf("GET %s: page should not claim that statistics are merely empty", path)
		}
	}
}

func TestPages_WithStatsStore_ShowData(t *testing.T) {
	store, err := stats.New(filepath.Join(t.TempDir(), "stats.db"))
	if err != nil {
		t.Fatalf("stats.New: %v", err)
	}
	defer store.Close()
	if err := store.RecordWrite("host/backup", 2048); err != nil {
		t.Fatalf("RecordWrite: %v", err)
	}

	for _, path := range []string{"/-/ui/", "/-/ui/repos/"} {
		body := serveUI(t, store, path)
		if strings.Contains(body, statsDisabledHint) {
			t.Errorf("GET %s: page should not show the stats disabled hint", path)
		}
		if !strings.Contains(body, "host/backup") {
			t.Errorf("GET %s: page should list repository host/backup", path)
		}
	}
}
