package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c-mueller/ts-restic-server/internal/apierror"
	"github.com/c-mueller/ts-restic-server/internal/middleware"
	"github.com/c-mueller/ts-restic-server/internal/storage/memory"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// parseErrorResponse decodes a response body into an ErrorResponse.
func parseErrorResponse(t *testing.T, rec *httptest.ResponseRecorder) apierror.ErrorResponse {
	t.Helper()
	var resp apierror.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse error response: %v\nbody: %s", err, rec.Body.String())
	}
	return resp
}

// setupTestHandler returns a Handler with an in-memory backend and nop logger.
func setupTestHandler(appendOnly bool) *Handler {
	backend := memory.New(0)
	return &Handler{
		Backend:    backend,
		Logger:     zap.NewNop(),
		AppendOnly: appendOnly,
	}
}

// newContext creates an echo.Context with a request ID set in the context.
func newContext(method, path string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, path, nil)
	ctx := context.WithValue(req.Context(), middleware.RequestIDKey, "test-request-id")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return c, rec
}

func TestErrorResponse_InvalidBlobType(t *testing.T) {
	h := setupTestHandler(false)
	c, rec := newContext(http.MethodGet, "/invalid/somename")
	c.SetParamNames("type", "name")
	c.SetParamValues("invalid", "somename")

	err := h.GetBlob(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	resp := parseErrorResponse(t, rec)
	if resp.Status != http.StatusBadRequest {
		t.Errorf("expected status 400 in body, got %d", resp.Status)
	}
	if resp.Error != "invalid blob type" {
		t.Errorf("expected error 'invalid blob type', got %q", resp.Error)
	}
	if resp.RequestID != "test-request-id" {
		t.Errorf("expected request_id 'test-request-id', got %q", resp.RequestID)
	}
}

func TestErrorResponse_BlobNotFound(t *testing.T) {
	h := setupTestHandler(false)
	// Init repo so it exists
	backend := h.Backend.(*memory.Backend)
	backend.CreateRepo(context.Background())

	c, rec := newContext(http.MethodGet, "/data/nonexistent")
	c.SetParamNames("type", "name")
	c.SetParamValues("data", "nonexistent")

	err := h.GetBlob(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}

	resp := parseErrorResponse(t, rec)
	if resp.Status != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.Status)
	}
	if resp.Error != "not found" {
		t.Errorf("expected error 'not found', got %q", resp.Error)
	}
}

func TestErrorResponse_AppendOnlyForbidden(t *testing.T) {
	h := setupTestHandler(true)
	c, rec := newContext(http.MethodDelete, "/data/somefile")
	c.SetParamNames("type", "name")
	c.SetParamValues("data", "somefile")

	err := h.DeleteBlob(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}

	resp := parseErrorResponse(t, rec)
	if resp.Status != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", resp.Status)
	}
	if resp.Error != "forbidden" {
		t.Errorf("expected error 'forbidden', got %q", resp.Error)
	}
	if resp.Message == "" {
		t.Error("expected non-empty message for append-only denial")
	}
}

func TestErrorResponse_CreateRepoMissingParam(t *testing.T) {
	h := setupTestHandler(false)
	c, rec := newContext(http.MethodPost, "/")

	err := h.CreateRepo(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	resp := parseErrorResponse(t, rec)
	if resp.Status != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.Status)
	}
	if resp.RequestID != "test-request-id" {
		t.Errorf("expected request_id 'test-request-id', got %q", resp.RequestID)
	}
}

func TestErrorResponse_DeleteRepoAppendOnly(t *testing.T) {
	h := setupTestHandler(true)
	c, rec := newContext(http.MethodDelete, "/")

	err := h.DeleteRepo(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}

	resp := parseErrorResponse(t, rec)
	if resp.Status != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", resp.Status)
	}
	if resp.Error != "forbidden" {
		t.Errorf("expected error 'forbidden', got %q", resp.Error)
	}
}

func TestErrorResponse_QuotaExceeded(t *testing.T) {
	c, rec := newContext(http.MethodPost, "/data/testblob")

	// Directly test the error response format for quota exceeded
	apiError(c, http.StatusInsufficientStorage, "quota exceeded", "storage quota has been exceeded")

	if rec.Code != http.StatusInsufficientStorage {
		t.Fatalf("expected 507, got %d", rec.Code)
	}

	resp := parseErrorResponse(t, rec)
	if resp.Status != http.StatusInsufficientStorage {
		t.Errorf("expected status 507, got %d", resp.Status)
	}
	if resp.Error != "quota exceeded" {
		t.Errorf("expected error 'quota exceeded', got %q", resp.Error)
	}
}

func TestErrorResponse_NoRequestID(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/data/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// No request ID in context — field should be omitted
	apiError(c, http.StatusNotFound, "not found", "")

	resp := parseErrorResponse(t, rec)
	if resp.RequestID != "" {
		t.Errorf("expected empty request_id when not set, got %q", resp.RequestID)
	}

	// Also verify it's omitted from JSON
	var raw map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &raw)
	if _, exists := raw["request_id"]; exists {
		t.Error("request_id should be omitted from JSON when empty")
	}
}

func TestErrorResponse_EmptyMessageOmitted(t *testing.T) {
	c, rec := newContext(http.MethodGet, "/test")
	apiError(c, http.StatusNotFound, "not found", "")

	var raw map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &raw)
	if _, exists := raw["message"]; exists {
		t.Error("message should be omitted from JSON when empty")
	}
}

func TestErrorResponse_DataOmitted(t *testing.T) {
	c, rec := newContext(http.MethodGet, "/test")
	apiError(c, http.StatusNotFound, "not found", "")

	var raw map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &raw)
	if _, exists := raw["data"]; exists {
		t.Error("data should be omitted from JSON when empty")
	}
}

func TestCustomHTTPErrorHandler_MethodNotAllowed(t *testing.T) {
	handler := apierror.CustomHTTPErrorHandler(func(c echo.Context) string {
		return middleware.GetRequestID(c.Request().Context())
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/unknown", nil)
	ctx := context.WithValue(req.Context(), middleware.RequestIDKey, "test-req-123")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler(echo.NewHTTPError(http.StatusMethodNotAllowed, "PUT method is not allowed"), c)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}

	resp := parseErrorResponse(t, rec)
	if resp.Status != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", resp.Status)
	}
	if resp.Error != "Method Not Allowed" {
		t.Errorf("expected error 'Method Not Allowed', got %q", resp.Error)
	}
	if resp.RequestID != "test-req-123" {
		t.Errorf("expected request_id 'test-req-123', got %q", resp.RequestID)
	}
}

func TestCustomHTTPErrorHandler_NotFound(t *testing.T) {
	handler := apierror.CustomHTTPErrorHandler(func(c echo.Context) string {
		return ""
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler(echo.ErrNotFound, c)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}

	resp := parseErrorResponse(t, rec)
	if resp.Status != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.Status)
	}
	if resp.Error != "Not Found" {
		t.Errorf("expected error 'Not Found', got %q", resp.Error)
	}
}

func TestCustomHTTPErrorHandler_HeadRequest(t *testing.T) {
	handler := apierror.CustomHTTPErrorHandler(func(c echo.Context) string {
		return ""
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodHead, "/nonexistent", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler(echo.ErrNotFound, c)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}

	// HEAD responses should have no body
	if rec.Body.Len() > 0 {
		t.Errorf("HEAD response should have no body, got %q", rec.Body.String())
	}
}

// Verify that error responses have the correct Content-Type.
func TestErrorResponse_ContentType(t *testing.T) {
	c, rec := newContext(http.MethodGet, "/test")
	apiError(c, http.StatusBadRequest, "bad request", "test message")

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json; charset=UTF-8" && ct != "application/json" {
		t.Errorf("expected Content-Type containing 'application/json', got %q", ct)
	}
}

// Verify ErrorResponse struct fields and JSON schema.
func TestErrorResponse_FullSchema(t *testing.T) {
	c, rec := newContext(http.MethodGet, "/test")
	apierror.WithData(c, http.StatusForbidden, "access denied", "you shall not pass", "test-request-id", map[string]string{
		"ip": "10.0.0.1",
	})

	var raw map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	expectedKeys := []string{"status", "error", "message", "request_id", "data"}
	for _, key := range expectedKeys {
		if _, exists := raw[key]; !exists {
			t.Errorf("missing key %q in error response", key)
		}
	}

	if raw["status"].(float64) != 403 {
		t.Errorf("expected status 403, got %v", raw["status"])
	}
	if raw["error"].(string) != "access denied" {
		t.Errorf("expected error 'access denied', got %v", raw["error"])
	}
	if raw["message"].(string) != "you shall not pass" {
		t.Errorf("expected message 'you shall not pass', got %v", raw["message"])
	}
	if raw["request_id"].(string) != "test-request-id" {
		t.Errorf("expected request_id 'test-request-id', got %v", raw["request_id"])
	}

	data, ok := raw["data"].(map[string]interface{})
	if !ok {
		t.Fatal("expected data to be a map")
	}
	if data["ip"].(string) != "10.0.0.1" {
		t.Errorf("expected data.ip '10.0.0.1', got %v", data["ip"])
	}
}

// Ensure the list endpoint returns errors in the standard format too.
func TestErrorResponse_ListInvalidType(t *testing.T) {
	h := setupTestHandler(false)
	c, rec := newContext(http.MethodGet, "/invalid/")
	c.SetParamNames("type")
	c.SetParamValues("invalid")

	err := h.ListBlobs(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	resp := parseErrorResponse(t, rec)
	if resp.Status != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.Status)
	}
	if resp.Error != "invalid blob type" {
		t.Errorf("expected error 'invalid blob type', got %q", resp.Error)
	}
}

// Ensure DELETE on non-existent blob returns standard error.
func TestErrorResponse_DeleteBlobNotFound(t *testing.T) {
	h := setupTestHandler(false)
	backend := h.Backend.(*memory.Backend)
	backend.CreateRepo(context.Background())

	c, rec := newContext(http.MethodDelete, "/data/nonexistent")
	c.SetParamNames("type", "name")
	c.SetParamValues("data", "nonexistent")

	err := h.DeleteBlob(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}

	resp := parseErrorResponse(t, rec)
	if resp.Status != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.Status)
	}
}

// Ensure config GET 404 returns standard error.
func TestErrorResponse_GetConfigNotFound(t *testing.T) {
	h := setupTestHandler(false)
	backend := h.Backend.(*memory.Backend)
	backend.CreateRepo(context.Background())

	c, rec := newContext(http.MethodGet, "/config")

	err := h.GetConfig(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}

	resp := parseErrorResponse(t, rec)
	if resp.Status != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.Status)
	}
}
