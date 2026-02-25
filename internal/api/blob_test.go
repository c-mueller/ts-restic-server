package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c-mueller/ts-restic-server/internal/storage"
	"github.com/labstack/echo/v4"
)

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
