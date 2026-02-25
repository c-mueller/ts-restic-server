package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func newContextWithAccept(accept string) echo.Context {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/data/", nil)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec)
}

func TestIsV2_CorrectCasing(t *testing.T) {
	c := newContextWithAccept("application/vnd.x.restic.rest.v2")
	if !isV2(c) {
		t.Error("expected isV2=true for correct casing")
	}
}

func TestIsV2_UpperCase(t *testing.T) {
	c := newContextWithAccept("APPLICATION/VND.X.RESTIC.REST.V2")
	if !isV2(c) {
		t.Error("expected isV2=true for uppercase Accept header")
	}
}

func TestIsV2_MixedCase(t *testing.T) {
	c := newContextWithAccept("Application/Vnd.X.Restic.Rest.V2")
	if !isV2(c) {
		t.Error("expected isV2=true for mixed case Accept header")
	}
}

func TestIsV2_V1Header(t *testing.T) {
	c := newContextWithAccept("application/vnd.x.restic.rest.v1")
	if isV2(c) {
		t.Error("expected isV2=false for V1 Accept header")
	}
}

func TestIsV2_EmptyHeader(t *testing.T) {
	c := newContextWithAccept("")
	if isV2(c) {
		t.Error("expected isV2=false for empty Accept header")
	}
}

func TestIsV2_DifferentType(t *testing.T) {
	c := newContextWithAccept("application/json")
	if isV2(c) {
		t.Error("expected isV2=false for unrelated Accept header")
	}
}
