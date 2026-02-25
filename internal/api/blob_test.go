package api

import (
	"net/http"
	"testing"
)

func TestIsValidBlobName(t *testing.T) {
	valid := []string{
		"abcdef0123456789",
		"ABCDEF0123456789",
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", // pragma: allowlist secret
		"aa",
		"0",
		"aAbBcCdDeEfF", // pragma: allowlist secret
	}
	for _, name := range valid {
		if !isValidBlobName(name) {
			t.Errorf("isValidBlobName(%q) = false, want true", name)
		}
	}

	invalid := []string{
		"",
		"../etc/passwd",
		"abc/def",
		"abc.def",
		"name with spaces",
		"abcdefg",    // 'g' is not hex
		"abcdef\x00", // null byte
		"../../../../../../etc/passwd",
		"hello-world",
		"abc_def",
	}
	for _, name := range invalid {
		if isValidBlobName(name) {
			t.Errorf("isValidBlobName(%q) = true, want false", name)
		}
	}
}

func TestErrorResponse_InvalidBlobName_Head(t *testing.T) {
	h := setupTestHandler(false)
	c, rec := newContext(http.MethodHead, "/data/../etc/passwd")
	c.SetParamNames("type", "name")
	c.SetParamValues("data", "../etc/passwd")

	err := h.HeadBlob(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	resp := parseErrorResponse(t, rec)
	if resp.Error != "invalid blob name" {
		t.Errorf("expected error 'invalid blob name', got %q", resp.Error)
	}
	if resp.Message != "blob name must be a hex string" {
		t.Errorf("expected message 'blob name must be a hex string', got %q", resp.Message)
	}
}

func TestErrorResponse_InvalidBlobName_Get(t *testing.T) {
	h := setupTestHandler(false)
	c, rec := newContext(http.MethodGet, "/data/not-hex-name")
	c.SetParamNames("type", "name")
	c.SetParamValues("data", "not-hex-name")

	err := h.GetBlob(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	resp := parseErrorResponse(t, rec)
	if resp.Error != "invalid blob name" {
		t.Errorf("expected error 'invalid blob name', got %q", resp.Error)
	}
}

func TestErrorResponse_InvalidBlobName_Save(t *testing.T) {
	h := setupTestHandler(false)
	c, rec := newContext(http.MethodPost, "/data/path.traversal")
	c.SetParamNames("type", "name")
	c.SetParamValues("data", "path.traversal")

	err := h.SaveBlob(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	resp := parseErrorResponse(t, rec)
	if resp.Error != "invalid blob name" {
		t.Errorf("expected error 'invalid blob name', got %q", resp.Error)
	}
}

func TestErrorResponse_InvalidBlobName_Delete(t *testing.T) {
	h := setupTestHandler(false)
	c, rec := newContext(http.MethodDelete, "/locks/../../escape")
	c.SetParamNames("type", "name")
	c.SetParamValues("locks", "../../escape")

	err := h.DeleteBlob(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	resp := parseErrorResponse(t, rec)
	if resp.Error != "invalid blob name" {
		t.Errorf("expected error 'invalid blob name', got %q", resp.Error)
	}
}

func TestValidBlobName_Passes(t *testing.T) {
	h := setupTestHandler(false)
	// Valid hex name, blob type check passes, but blob doesn't exist → 404 (not 400)
	c, rec := newContext(http.MethodHead, "/data/abcdef0123456789")
	c.SetParamNames("type", "name")
	c.SetParamValues("data", "abcdef0123456789")

	err := h.HeadBlob(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should get 404 (not found), not 400 (bad name)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for valid hex name that doesn't exist, got %d", rec.Code)
	}
}
