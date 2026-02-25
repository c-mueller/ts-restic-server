package apierror

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestNew_BasicResponse(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	New(c, http.StatusBadRequest, "bad request", "invalid input", "req-123")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json; charset=UTF-8" && ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != http.StatusBadRequest {
		t.Errorf("resp.Status = %d, want %d", resp.Status, http.StatusBadRequest)
	}
	if resp.Error != "bad request" {
		t.Errorf("resp.Error = %q, want %q", resp.Error, "bad request")
	}
	if resp.Message != "invalid input" {
		t.Errorf("resp.Message = %q, want %q", resp.Message, "invalid input")
	}
	if resp.RequestID != "req-123" {
		t.Errorf("resp.RequestID = %q, want %q", resp.RequestID, "req-123")
	}
}

func TestNew_OmitsEmptyMessage(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	New(c, http.StatusNotFound, "not found", "", "req-456")

	var raw map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &raw)
	if _, exists := raw["message"]; exists {
		t.Error("message should be omitted when empty")
	}
}

func TestNew_OmitsEmptyRequestID(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	New(c, http.StatusNotFound, "not found", "", "")

	var raw map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &raw)
	if _, exists := raw["request_id"]; exists {
		t.Error("request_id should be omitted when empty")
	}
}

func TestWithData_IncludesData(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	data := map[string]string{"ip": "10.0.0.1"}
	WithData(c, http.StatusForbidden, "forbidden", "access denied", "req-789", data)

	var raw map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &raw)

	d, ok := raw["data"].(map[string]interface{})
	if !ok {
		t.Fatal("expected data to be a map")
	}
	if d["ip"] != "10.0.0.1" {
		t.Errorf("data.ip = %v, want 10.0.0.1", d["ip"])
	}
}

func TestWithData_NilDataOmitted(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	WithData(c, http.StatusForbidden, "forbidden", "msg", "req-000", nil)

	var raw map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &raw)
	if _, exists := raw["data"]; exists {
		t.Error("data should be omitted when nil")
	}
}

func TestCustomHTTPErrorHandler_NonHTTPError(t *testing.T) {
	handler := CustomHTTPErrorHandler(func(c echo.Context) string { return "" })

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler(errors.New("plain error"), c)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	var resp ErrorResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Status != http.StatusInternalServerError {
		t.Errorf("resp.Status = %d, want %d", resp.Status, http.StatusInternalServerError)
	}
	if resp.Error != "Internal Server Error" {
		t.Errorf("resp.Error = %q, want %q", resp.Error, "Internal Server Error")
	}
}

func TestCustomHTTPErrorHandler_NilRequestIDFunc(t *testing.T) {
	handler := CustomHTTPErrorHandler(nil)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Should not panic
	handler(echo.ErrNotFound, c)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	var resp ErrorResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.RequestID != "" {
		t.Errorf("request_id should be empty with nil func, got %q", resp.RequestID)
	}
}

func TestCustomHTTPErrorHandler_CommittedResponse(t *testing.T) {
	handler := CustomHTTPErrorHandler(func(c echo.Context) string { return "req-id" })

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Simulate a committed response
	c.Response().Committed = true

	handler(echo.ErrNotFound, c)

	// Body should be empty since we returned early
	if rec.Body.Len() > 0 {
		t.Errorf("expected empty body for committed response, got %q", rec.Body.String())
	}
}
