package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c-mueller/ts-restic-server/internal/storage"
	"github.com/labstack/echo/v4"
)

// --- parseRange tests ---

func newContextWithRange(rangeHeader string) echo.Context {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/data/abc123", nil)
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec)
}

func TestParseRange_ValidFullRange(t *testing.T) {
	c := newContextWithRange("bytes=0-99")
	offset, length, ok := parseRange(c)
	if !ok {
		t.Fatal("expected rangeRequested=true")
	}
	if offset != 0 {
		t.Errorf("offset: got %d, want 0", offset)
	}
	if length != 100 {
		t.Errorf("length: got %d, want 100", length)
	}
}

func TestParseRange_ValidMiddleRange(t *testing.T) {
	c := newContextWithRange("bytes=100-199")
	offset, length, ok := parseRange(c)
	if !ok {
		t.Fatal("expected rangeRequested=true")
	}
	if offset != 100 {
		t.Errorf("offset: got %d, want 100", offset)
	}
	if length != 100 {
		t.Errorf("length: got %d, want 100", length)
	}
}

func TestParseRange_OpenEnd(t *testing.T) {
	c := newContextWithRange("bytes=100-")
	offset, length, ok := parseRange(c)
	if !ok {
		t.Fatal("expected rangeRequested=true")
	}
	if offset != 100 {
		t.Errorf("offset: got %d, want 100", offset)
	}
	if length != 0 {
		t.Errorf("length: got %d, want 0 (to EOF)", length)
	}
}

func TestParseRange_NegativeStart(t *testing.T) {
	c := newContextWithRange("bytes=-100-5")
	_, _, ok := parseRange(c)
	if ok {
		t.Error("expected rangeRequested=false for negative start")
	}
}

func TestParseRange_NegativeEnd(t *testing.T) {
	c := newContextWithRange("bytes=100--5")
	_, _, ok := parseRange(c)
	if ok {
		t.Error("expected rangeRequested=false for negative end")
	}
}

func TestParseRange_StartGreaterThanEnd(t *testing.T) {
	c := newContextWithRange("bytes=200-100")
	_, _, ok := parseRange(c)
	if ok {
		t.Error("expected rangeRequested=false when start > end")
	}
}

func TestParseRange_NoHeader(t *testing.T) {
	c := newContextWithRange("")
	_, _, ok := parseRange(c)
	if ok {
		t.Error("expected rangeRequested=false for empty header")
	}
}

func TestParseRange_InvalidPrefix(t *testing.T) {
	c := newContextWithRange("pages=0-99")
	_, _, ok := parseRange(c)
	if ok {
		t.Error("expected rangeRequested=false for invalid prefix")
	}
}

func TestParseRange_MalformedGarbage(t *testing.T) {
	c := newContextWithRange("bytes=abc-def")
	_, _, ok := parseRange(c)
	if ok {
		t.Error("expected rangeRequested=false for non-numeric values")
	}
}

func TestParseRange_SuffixRange(t *testing.T) {
	// "bytes=-500" is a suffix range (last 500 bytes) per RFC 7233,
	// but our parser requires an explicit start, so it should reject this.
	c := newContextWithRange("bytes=-500")
	_, _, ok := parseRange(c)
	if ok {
		t.Error("expected rangeRequested=false for suffix range without start")
	}
}

// --- blobParams tests ---

func newContextWithParam(paramType, paramName string) echo.Context {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("type", "name")
	c.SetParamValues(paramType, paramName)
	return c
}

func TestBlobParams_Lowercase(t *testing.T) {
	c := newContextWithParam("data", "abc123")
	bt, name := blobParams(c)
	if bt != storage.BlobData {
		t.Errorf("blobType: got %q, want %q", bt, storage.BlobData)
	}
	if name != "abc123" {
		t.Errorf("name: got %q, want %q", name, "abc123")
	}
}

func TestBlobParams_Uppercase(t *testing.T) {
	c := newContextWithParam("Data", "abc123")
	bt, _ := blobParams(c)
	if bt != storage.BlobData {
		t.Errorf("blobType: got %q, want %q (should normalize uppercase)", bt, storage.BlobData)
	}
}

func TestBlobParams_MixedCase(t *testing.T) {
	c := newContextWithParam("Keys", "abc123")
	bt, _ := blobParams(c)
	if bt != storage.BlobKeys {
		t.Errorf("blobType: got %q, want %q (should normalize mixed case)", bt, storage.BlobKeys)
	}
}

func TestBlobParams_AllCaps(t *testing.T) {
	c := newContextWithParam("SNAPSHOTS", "abc123")
	bt, _ := blobParams(c)
	if bt != storage.BlobSnapshots {
		t.Errorf("blobType: got %q, want %q (should normalize all caps)", bt, storage.BlobSnapshots)
	}
}

// --- isValidBlobName tests ---

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
