package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c-mueller/ts-restic-server/internal/middleware"
	"github.com/c-mueller/ts-restic-server/internal/storage"
	"github.com/c-mueller/ts-restic-server/internal/storage/memory"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// setupTestHandlerWithQuota creates a Handler backed by a memory backend with the given quota.
func setupTestHandlerWithQuota(appendOnly bool, maxBytes int64) *Handler {
	backend := memory.New(maxBytes)
	return &Handler{
		Backend:    backend,
		Logger:     zap.NewNop(),
		AppendOnly: appendOnly,
	}
}

// newContextWithBody creates an echo.Context with a request body, request ID, and repo prefix.
func newContextWithBody(method, path string, body []byte) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	ctx := context.WithValue(req.Context(), middleware.RequestIDKey, "test-request-id")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return c, rec
}

// initRepo creates a repo via the backend for tests that need it.
func initRepo(t *testing.T, h *Handler) {
	t.Helper()
	if err := h.Backend.CreateRepo(context.Background()); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}
}

// --- Repo Management ---

func TestCreateRepo_Success(t *testing.T) {
	h := setupTestHandlerWithQuota(false, 1024*1024)
	c, rec := newContext(http.MethodPost, "/?create=true")
	if err := h.CreateRepo(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestCreateRepo_AlreadyExists(t *testing.T) {
	h := setupTestHandlerWithQuota(false, 1024*1024)

	c1, _ := newContext(http.MethodPost, "/?create=true")
	h.CreateRepo(c1)

	c2, rec := newContext(http.MethodPost, "/?create=true")
	if err := h.CreateRepo(c2); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestDeleteRepo_Success(t *testing.T) {
	h := setupTestHandlerWithQuota(false, 1024*1024)
	initRepo(t, h)

	c, rec := newContext(http.MethodDelete, "/")
	if err := h.DeleteRepo(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestDeleteRepo_NotFound(t *testing.T) {
	h := setupTestHandlerWithQuota(false, 1024*1024)

	c, rec := newContext(http.MethodDelete, "/")
	if err := h.DeleteRepo(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// --- Config Endpoints ---

func TestHeadConfig_Success(t *testing.T) {
	h := setupTestHandlerWithQuota(false, 1024*1024)
	initRepo(t, h)

	data := []byte(`{"version":2}`)
	h.Backend.SaveConfig(context.Background(), bytes.NewReader(data))

	c, rec := newContext(http.MethodHead, "/config")
	if err := h.HeadConfig(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if cl := rec.Header().Get("Content-Length"); cl != "13" {
		t.Fatalf("Content-Length = %q, want %q", cl, "13")
	}
}

func TestHeadConfig_NotFound(t *testing.T) {
	h := setupTestHandlerWithQuota(false, 1024*1024)
	initRepo(t, h)

	c, rec := newContext(http.MethodHead, "/config")
	if err := h.HeadConfig(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetConfig_Success(t *testing.T) {
	h := setupTestHandlerWithQuota(false, 1024*1024)
	initRepo(t, h)

	data := []byte(`{"version":2}`)
	h.Backend.SaveConfig(context.Background(), bytes.NewReader(data))

	c, rec := newContext(http.MethodGet, "/config")
	if err := h.GetConfig(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !bytes.Equal(rec.Body.Bytes(), data) {
		t.Fatalf("body = %q, want %q", rec.Body.String(), data)
	}
}

func TestSaveConfig_Success(t *testing.T) {
	h := setupTestHandlerWithQuota(false, 1024*1024)
	initRepo(t, h)

	data := []byte(`{"version":3}`)
	c, rec := newContextWithBody(http.MethodPost, "/config", data)
	if err := h.SaveConfig(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Verify via backend
	size, err := h.Backend.StatConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if size != int64(len(data)) {
		t.Fatalf("StatConfig size = %d, want %d", size, len(data))
	}
}

// --- Blob Endpoints ---

func TestHeadBlob_Success(t *testing.T) {
	h := setupTestHandlerWithQuota(false, 1024*1024)
	initRepo(t, h)

	data := []byte("blob-content-for-head")
	h.Backend.SaveBlob(context.Background(), storage.BlobData, "aabb0011", bytes.NewReader(data))

	c, rec := newContext(http.MethodHead, "/data/aabb0011")
	c.SetParamNames("type", "name")
	c.SetParamValues("data", "aabb0011")
	if err := h.HeadBlob(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if cl := rec.Header().Get("Content-Length"); cl != "21" {
		t.Fatalf("Content-Length = %q, want %q", cl, "21")
	}
	if ar := rec.Header().Get("Accept-Ranges"); ar != "bytes" {
		t.Fatalf("Accept-Ranges = %q, want %q", ar, "bytes")
	}
}

func TestGetBlob_Success(t *testing.T) {
	h := setupTestHandlerWithQuota(false, 1024*1024)
	initRepo(t, h)

	data := []byte("full-blob-content-here")
	h.Backend.SaveBlob(context.Background(), storage.BlobData, "aabb0022", bytes.NewReader(data))

	c, rec := newContext(http.MethodGet, "/data/aabb0022")
	c.SetParamNames("type", "name")
	c.SetParamValues("data", "aabb0022")
	if err := h.GetBlob(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Fatalf("Content-Type = %q, want application/octet-stream", ct)
	}
	if !bytes.Equal(rec.Body.Bytes(), data) {
		t.Fatalf("body = %q, want %q", rec.Body.String(), data)
	}
}

func TestGetBlob_Range(t *testing.T) {
	h := setupTestHandlerWithQuota(false, 1024*1024)
	initRepo(t, h)

	// 100 bytes of data
	data := bytes.Repeat([]byte("x"), 100)
	h.Backend.SaveBlob(context.Background(), storage.BlobData, "aabb0033", bytes.NewReader(data))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/data/aabb0033", nil)
	req.Header.Set("Range", "bytes=0-49")
	ctx := context.WithValue(req.Context(), middleware.RequestIDKey, "test-request-id")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("type", "name")
	c.SetParamValues("data", "aabb0033")

	if err := h.GetBlob(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusPartialContent)
	}
	if cr := rec.Header().Get("Content-Range"); cr != "bytes 0-49/100" {
		t.Fatalf("Content-Range = %q, want %q", cr, "bytes 0-49/100")
	}
	if rec.Body.Len() != 50 {
		t.Fatalf("body len = %d, want 50", rec.Body.Len())
	}
}

func TestGetBlob_RangeOpenEnd(t *testing.T) {
	h := setupTestHandlerWithQuota(false, 1024*1024)
	initRepo(t, h)

	data := bytes.Repeat([]byte("y"), 100)
	h.Backend.SaveBlob(context.Background(), storage.BlobData, "aabb0044", bytes.NewReader(data))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/data/aabb0044", nil)
	req.Header.Set("Range", "bytes=50-")
	ctx := context.WithValue(req.Context(), middleware.RequestIDKey, "test-request-id")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("type", "name")
	c.SetParamValues("data", "aabb0044")

	if err := h.GetBlob(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusPartialContent)
	}
	if rec.Body.Len() != 50 {
		t.Fatalf("body len = %d, want 50", rec.Body.Len())
	}
}

func TestGetBlob_RangeNotSatisfiable(t *testing.T) {
	h := setupTestHandlerWithQuota(false, 1024*1024)
	initRepo(t, h)

	data := []byte("short")
	h.Backend.SaveBlob(context.Background(), storage.BlobData, "aabb0055", bytes.NewReader(data))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/data/aabb0055", nil)
	req.Header.Set("Range", "bytes=100-")
	ctx := context.WithValue(req.Context(), middleware.RequestIDKey, "test-request-id")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("type", "name")
	c.SetParamValues("data", "aabb0055")

	if err := h.GetBlob(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestedRangeNotSatisfiable)
	}
}

func TestSaveBlob_Success(t *testing.T) {
	h := setupTestHandlerWithQuota(false, 1024*1024)
	initRepo(t, h)

	data := []byte("new-blob-data")
	c, rec := newContextWithBody(http.MethodPost, "/data/aabb0066", data)
	c.SetParamNames("type", "name")
	c.SetParamValues("data", "aabb0066")

	if err := h.SaveBlob(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Verify via backend
	size, err := h.Backend.StatBlob(context.Background(), storage.BlobData, "aabb0066")
	if err != nil {
		t.Fatal(err)
	}
	if size != int64(len(data)) {
		t.Fatalf("StatBlob size = %d, want %d", size, len(data))
	}
}

func TestDeleteBlob_Success(t *testing.T) {
	h := setupTestHandlerWithQuota(false, 1024*1024)
	initRepo(t, h)

	h.Backend.SaveBlob(context.Background(), storage.BlobData, "aabb0077", bytes.NewReader([]byte("data")))

	c, rec := newContext(http.MethodDelete, "/data/aabb0077")
	c.SetParamNames("type", "name")
	c.SetParamValues("data", "aabb0077")

	if err := h.DeleteBlob(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestDeleteBlob_AppendOnly_LocksAllowed(t *testing.T) {
	h := setupTestHandlerWithQuota(true, 1024*1024)
	initRepo(t, h)

	h.Backend.SaveBlob(context.Background(), storage.BlobLocks, "aabb0088", bytes.NewReader([]byte("lock")))

	c, rec := newContext(http.MethodDelete, "/locks/aabb0088")
	c.SetParamNames("type", "name")
	c.SetParamValues("locks", "aabb0088")

	if err := h.DeleteBlob(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// --- List Endpoints ---

func TestListBlobs_V1_Empty(t *testing.T) {
	h := setupTestHandlerWithQuota(false, 1024*1024)
	initRepo(t, h)

	c, rec := newContext(http.MethodGet, "/data/")
	c.SetParamNames("type")
	c.SetParamValues("data")

	if err := h.ListBlobs(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var names []string
	json.Unmarshal(rec.Body.Bytes(), &names)
	if len(names) != 0 {
		t.Fatalf("expected empty array, got %v", names)
	}
}

func TestListBlobs_V1_WithData(t *testing.T) {
	h := setupTestHandlerWithQuota(false, 1024*1024)
	initRepo(t, h)

	for _, name := range []string{"aa01", "bb02", "cc03"} {
		h.Backend.SaveBlob(context.Background(), storage.BlobData, name, bytes.NewReader([]byte("d")))
	}

	c, rec := newContext(http.MethodGet, "/data/")
	c.SetParamNames("type")
	c.SetParamValues("data")

	if err := h.ListBlobs(c); err != nil {
		t.Fatal(err)
	}

	var names []string
	json.Unmarshal(rec.Body.Bytes(), &names)
	if len(names) != 3 {
		t.Fatalf("expected 3 names, got %d: %v", len(names), names)
	}
}

func TestListBlobs_V2_Empty(t *testing.T) {
	h := setupTestHandlerWithQuota(false, 1024*1024)
	initRepo(t, h)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/data/", nil)
	req.Header.Set("Accept", resticV2Type)
	ctx := context.WithValue(req.Context(), middleware.RequestIDKey, "test-request-id")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("type")
	c.SetParamValues("data")

	if err := h.ListBlobs(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var blobs []storage.Blob
	json.Unmarshal(rec.Body.Bytes(), &blobs)
	if len(blobs) != 0 {
		t.Fatalf("expected empty array, got %v", blobs)
	}
}

func TestListBlobs_V2_WithData(t *testing.T) {
	h := setupTestHandlerWithQuota(false, 1024*1024)
	initRepo(t, h)

	items := map[string][]byte{
		"aa01": []byte("one"),
		"bb02": []byte("twotwo"),
		"cc03": []byte("threethreethree"),
	}
	for name, data := range items {
		h.Backend.SaveBlob(context.Background(), storage.BlobData, name, bytes.NewReader(data))
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/data/", nil)
	req.Header.Set("Accept", resticV2Type)
	ctx := context.WithValue(req.Context(), middleware.RequestIDKey, "test-request-id")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("type")
	c.SetParamValues("data")

	if err := h.ListBlobs(c); err != nil {
		t.Fatal(err)
	}

	var blobs []storage.Blob
	json.Unmarshal(rec.Body.Bytes(), &blobs)
	if len(blobs) != 3 {
		t.Fatalf("expected 3 blobs, got %d", len(blobs))
	}

	for _, blob := range blobs {
		wantSize := int64(len(items[blob.Name]))
		if blob.Size != wantSize {
			t.Errorf("blob %s size = %d, want %d", blob.Name, blob.Size, wantSize)
		}
	}
}
