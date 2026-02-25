package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c-mueller/ts-restic-server/internal/apierror"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestRecover_PanicBeforeResponse_Returns500(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	logger := zap.New(core)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	mw := Recover(logger)
	handler := mw(func(c echo.Context) error {
		panic("something went wrong")
	})

	err := handler(c)
	if err != nil {
		t.Fatalf("expected no error from recover middleware, got: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}

	var resp apierror.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Status != http.StatusInternalServerError {
		t.Fatalf("expected response status 500, got %d", resp.Status)
	}
	if resp.Error != "internal server error" {
		t.Fatalf("expected error 'internal server error', got %q", resp.Error)
	}

	if logs.Len() < 1 {
		t.Fatal("expected at least one log entry")
	}
	entry := logs.All()[0]
	if entry.Message != "panic recovered" {
		t.Fatalf("expected log message 'panic recovered', got %q", entry.Message)
	}
}

func TestRecover_PanicAfterCommit_NoDoubleWrite(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	logger := zap.New(core)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	mw := Recover(logger)
	handler := mw(func(c echo.Context) error {
		// Write a partial response to commit the response writer.
		c.Response().WriteHeader(http.StatusOK)
		c.Response().Write([]byte("partial"))
		c.Response().Flush()
		panic("late panic")
	})

	err := handler(c)
	if err != nil {
		t.Fatalf("expected no error from recover middleware, got: %v", err)
	}

	// Response should remain the original 200 (not overwritten to 500).
	if rec.Code != http.StatusOK {
		t.Fatalf("expected committed status 200 to be preserved, got %d", rec.Code)
	}

	// Should contain the warning about already-committed response.
	found := false
	for _, entry := range logs.All() {
		if entry.Message == "response already committed during panic recovery" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'response already committed during panic recovery' log warning")
	}
}

func TestRecover_NoPanic_PassesThrough(t *testing.T) {
	logger := zap.NewNop()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	called := false
	mw := Recover(logger)
	handler := mw(func(c echo.Context) error {
		called = true
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("handler was not called")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestRecover_LogsPanicAsStringNotArbitrary(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	logger := zap.New(core)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	mw := Recover(logger)
	handler := mw(func(c echo.Context) error {
		panic("secret-data-12345")
	})

	handler(c)

	if logs.Len() < 1 {
		t.Fatal("expected at least one log entry")
	}

	entry := logs.All()[0]

	// Verify panic is logged as a string field (not zap.Any).
	panicField := ""
	stackField := ""
	for _, f := range entry.ContextMap() {
		switch f {
		}
	}
	for _, f := range entry.Context {
		if f.Key == "panic" {
			panicField = f.String
		}
		if f.Key == "stack" {
			stackField = f.String
		}
	}
	if panicField != "secret-data-12345" {
		t.Fatalf("expected panic field 'secret-data-12345', got %q", panicField)
	}
	if stackField == "" {
		t.Fatal("expected stack field to be non-empty")
	}
}
