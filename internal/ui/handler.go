package ui

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/c-mueller/ts-restic-server/internal/stats"
	"github.com/labstack/echo/v4"
)

// Handler serves the Web UI pages.
type Handler struct {
	store     *stats.Store
	templates map[string]*template.Template
}

// NewHandler creates a UI handler. If store is nil, pages show
// a "stats not enabled" message.
func NewHandler(store *stats.Store) (*Handler, error) {
	h := &Handler{store: store}
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

	h.templates = make(map[string]*template.Template)
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
		h.templates[page] = tmpl
	}
	return nil
}

func (h *Handler) render(c echo.Context, name string, data interface{}) error {
	tmpl, ok := h.templates[name]
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "template not found: "+name)
	}
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	c.Response().WriteHeader(http.StatusOK)
	return tmpl.ExecuteTemplate(c.Response(), "layout", data)
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

// RepoList shows all repositories with stats.
func (h *Handler) RepoList(c echo.Context) error {
	data := map[string]interface{}{
		"Active": "repos",
		"Stats":  []stats.RepoStats(nil),
	}

	if h.store != nil {
		if all, err := h.store.GetAllRepoStats(); err == nil {
			data["Stats"] = all
		}
	}

	return h.render(c, "repos.html", data)
}

// RepoDetail shows stats for a single repository.
func (h *Handler) RepoDetail(c echo.Context) error {
	repoPath := c.Param("*")
	repoPath = strings.TrimSuffix(repoPath, "/")

	data := map[string]interface{}{
		"Active":   "repos",
		"Repo":     &stats.RepoStats{RepoPath: repoPath},
		"TotalOps": int64(0),
	}

	if h.store != nil {
		if rs, err := h.store.GetRepoStats(repoPath); err == nil && rs != nil {
			data["Repo"] = rs
			data["TotalOps"] = rs.WriteCount + rs.ReadCount + rs.DeleteCount
		}
	}

	return h.render(c, "repo_detail.html", data)
}

// RegisterRoutes registers the UI routes on the Echo instance.
// If auth is configured (non-empty username), Basic Auth is applied.
func RegisterRoutes(e *echo.Echo, store *stats.Store, authUser, authPass string) error {
	h, err := NewHandler(store)
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
