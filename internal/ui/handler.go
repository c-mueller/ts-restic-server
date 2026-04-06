package ui

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/c-mueller/ts-restic-server/internal/middleware"
	"github.com/c-mueller/ts-restic-server/internal/stats"
	"github.com/c-mueller/ts-restic-server/internal/storage"
	"github.com/labstack/echo/v4"
)

// Handler serves the Web UI pages.
type Handler struct {
	store   *stats.Store
	backend storage.Backend
	tmpls   map[string]*template.Template
}

// NewHandler creates a UI handler. If store is nil, pages show
// a "stats not enabled" message. backend is required for lock operations.
func NewHandler(store *stats.Store, backend storage.Backend) (*Handler, error) {
	h := &Handler{store: store, backend: backend}
	if err := h.loadTemplates(); err != nil {
		return nil, fmt.Errorf("load templates: %w", err)
	}
	return h, nil
}

var funcMap = template.FuncMap{
	"fmtBytes": formatBytes,
	"fmtTime":  formatTime,
}

func (h *Handler) loadTemplates() error {
	layoutBytes, err := templateFS.ReadFile("templates/layout.html")
	if err != nil {
		return err
	}
	layout := string(layoutBytes)

	h.tmpls = make(map[string]*template.Template)
	pages := []string{"dashboard.html", "repos.html", "repo_detail.html"}
	for _, page := range pages {
		pageBytes, err := templateFS.ReadFile("templates/" + page)
		if err != nil {
			return err
		}
		tmpl, err := template.New(page).Funcs(funcMap).Parse(layout + "\n" + string(pageBytes))
		if err != nil {
			return fmt.Errorf("parse %s: %w", page, err)
		}
		h.tmpls[page] = tmpl
	}
	return nil
}

func (h *Handler) render(c echo.Context, name string, data interface{}) error {
	tmpl, ok := h.tmpls[name]
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "template not found: "+name)
	}
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	c.Response().WriteHeader(http.StatusOK)
	return tmpl.ExecuteTemplate(c.Response(), "layout", data)
}

// repoCtx creates a context with the given repo prefix for backend operations.
func repoCtx(repoPath string) context.Context {
	return middleware.ContextWithRepoPrefix(context.Background(), repoPath)
}

// Dashboard shows aggregate stats.
func (h *Handler) Dashboard(c echo.Context) error {
	data := map[string]interface{}{
		"Active":    "dashboard",
		"RepoCount": 0,
		"Summary":   &stats.RepoStats{},
		"Stats":     []stats.RepoStats(nil),
	}

	if h.store != nil {
		if summary, err := h.store.GetSummary(); err == nil {
			data["Summary"] = summary
		}
		if all, err := h.store.GetAllRepoStats(); err == nil {
			data["Stats"] = all
			data["RepoCount"] = len(all)
		}
	}

	return h.render(c, "dashboard.html", data)
}

// repoWithLocks is used by the repos template to show lock counts.
type repoWithLocks struct {
	stats.RepoStats
	LockCount int
}

// RepoList shows all repositories with stats and lock counts.
func (h *Handler) RepoList(c echo.Context) error {
	data := map[string]interface{}{
		"Active": "repos",
		"Stats":  []repoWithLocks(nil),
	}

	if h.store != nil {
		if all, err := h.store.GetAllRepoStats(); err == nil {
			repos := make([]repoWithLocks, len(all))
			for i, rs := range all {
				repos[i] = repoWithLocks{RepoStats: rs}
				if h.backend != nil {
					if locks, err := h.backend.ListBlobs(repoCtx(rs.RepoPath), storage.BlobLocks); err == nil {
						repos[i].LockCount = len(locks)
					}
				}
			}
			data["Stats"] = repos
		}
	}

	return h.render(c, "repos.html", data)
}

// RepoDetail shows stats for a single repository including locks.
func (h *Handler) RepoDetail(c echo.Context) error {
	repoPath := c.Param("*")
	repoPath = strings.TrimSuffix(repoPath, "/")

	// Generate CSRF token for lock deletion forms.
	csrfToken := generateCSRFToken()
	c.SetCookie(&http.Cookie{
		Name:     "csrf_token",
		Value:    csrfToken,
		Path:     "/-/ui/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

	data := map[string]interface{}{
		"Active":    "repos",
		"Repo":      &stats.RepoStats{RepoPath: repoPath},
		"TotalOps":  int64(0),
		"Locks":     []storage.Blob(nil),
		"CSRFToken": csrfToken,
		"Flash":     c.QueryParam("msg"),
		"FlashType": c.QueryParam("type"),
	}

	if h.store != nil {
		if rs, err := h.store.GetRepoStats(repoPath); err == nil && rs != nil {
			data["Repo"] = rs
			data["TotalOps"] = rs.WriteCount + rs.ReadCount + rs.DeleteCount
		}
	}

	if h.backend != nil {
		if locks, err := h.backend.ListBlobs(repoCtx(repoPath), storage.BlobLocks); err == nil {
			data["Locks"] = locks
		}
	}

	return h.render(c, "repo_detail.html", data)
}

// DeleteLock handles POST requests to delete a lock blob.
func (h *Handler) DeleteLock(c echo.Context) error {
	repoPath := c.Param("*")
	repoPath = strings.TrimSuffix(repoPath, "/")
	lockName := c.FormValue("lock_name")
	formToken := c.FormValue("csrf_token")

	redirectURL := fmt.Sprintf("/-/ui/repos/%s/", repoPath)

	// Validate CSRF token.
	cookie, err := c.Cookie("csrf_token")
	if err != nil || cookie.Value == "" || cookie.Value != formToken {
		return c.Redirect(http.StatusSeeOther, redirectURL+"?msg=Invalid+CSRF+token&type=danger")
	}

	if lockName == "" {
		return c.Redirect(http.StatusSeeOther, redirectURL+"?msg=No+lock+specified&type=danger")
	}

	if h.backend == nil {
		return c.Redirect(http.StatusSeeOther, redirectURL+"?msg=Backend+not+available&type=danger")
	}

	if err := h.backend.DeleteBlob(repoCtx(repoPath), storage.BlobLocks, lockName); err != nil {
		msg := fmt.Sprintf("Failed+to+delete+lock:+%s", err.Error())
		return c.Redirect(http.StatusSeeOther, redirectURL+"?msg="+msg+"&type=danger")
	}

	return c.Redirect(http.StatusSeeOther, redirectURL+"?msg=Lock+deleted+successfully&type=success")
}

// RegisterRoutes registers the UI routes on the Echo instance.
// If auth is configured (non-empty username), Basic Auth is applied.
func RegisterRoutes(e *echo.Echo, store *stats.Store, backend storage.Backend, authUser, authPass string) error {
	h, err := NewHandler(store, backend)
	if err != nil {
		return err
	}

	// Serve embedded static assets.
	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return fmt.Errorf("static fs: %w", err)
	}
	staticHandler := http.StripPrefix("/-/ui/static/", http.FileServer(http.FS(staticSub)))

	uiGroup := e.Group("/-/ui")

	if authUser != "" {
		uiGroup.Use(basicAuth(authUser, authPass))
	}

	uiGroup.GET("/static/*", echo.WrapHandler(staticHandler))
	uiGroup.GET("/", h.Dashboard)
	uiGroup.GET("", func(c echo.Context) error {
		return c.Redirect(http.StatusMovedPermanently, "/-/ui/")
	})
	uiGroup.GET("/repos/", h.RepoList)
	uiGroup.GET("/repos/*", h.RepoDetail)
	uiGroup.POST("/repos/*/delete-lock", h.DeleteLock)

	return nil
}

func basicAuth(username, password string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			u, p, ok := c.Request().BasicAuth()
			if !ok || u != username || p != password {
				c.Response().Header().Set("WWW-Authenticate", `Basic realm="ts-restic-server"`)
				return c.String(http.StatusUnauthorized, "Unauthorized")
			}
			return next(c)
		}
	}
}

func generateCSRFToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func formatBytes(b int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
		TB = 1024 * GB
	)
	switch {
	case b >= TB:
		return fmt.Sprintf("%.1f TB", float64(b)/float64(TB))
	case b >= GB:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02 15:04:05")
}
