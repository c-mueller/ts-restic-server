package ui

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c-mueller/ts-restic-server/internal/stats"
	"github.com/c-mueller/ts-restic-server/internal/storage"
	"github.com/c-mueller/ts-restic-server/internal/storage/filesystem"
	"github.com/labstack/echo/v5"
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

func TestDeleteLock_OnlyDeletesLocksOfTheRepo(t *testing.T) {
	backend, err := filesystem.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := repoCtx("host/backup")
	if err := backend.CreateRepo(ctx); err != nil {
		t.Fatal(err)
	}
	for _, b := range []struct {
		t    storage.BlobType
		name string
	}{{storage.BlobKeys, "aaaa"}, {storage.BlobLocks, "bbbb"}} {
		if err := backend.SaveBlob(ctx, b.t, b.name, strings.NewReader("x")); err != nil {
			t.Fatal(err)
		}
	}

	e := echo.New()
	if err := RegisterRoutes(e, nil, backend, "", ""); err != nil {
		t.Fatal(err)
	}
	post := func(target, lock string) int {
		form := url.Values{"lock_name": {lock}, "csrf_token": {"token"}}
		req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "token"})
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec.Code
	}

	post("/-/ui/repos/host/backup/delete-lock", "../keys/aaaa")
	if code := post("/-/ui/repos/host/x/../backup/delete-lock", "bbbb"); code != http.StatusBadRequest {
		t.Errorf("repo path with ..: status = %d, want 400", code)
	}
	if _, err := backend.StatBlob(ctx, storage.BlobKeys, "aaaa"); err != nil {
		t.Errorf("key must survive a lock name that is not a lock: %v", err)
	}
	if _, err := backend.StatBlob(ctx, storage.BlobLocks, "bbbb"); err != nil {
		t.Fatalf("lock must survive a rejected repo path: %v", err)
	}

	if code := post("/-/ui/repos/host/backup/delete-lock", "bbbb"); code != http.StatusSeeOther {
		t.Fatalf("delete lock: status = %d, want 303", code)
	}
	if _, err := backend.StatBlob(ctx, storage.BlobLocks, "bbbb"); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("lock should be deleted, StatBlob error = %v", err)
	}
}
