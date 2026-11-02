package cmd

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c-mueller/ts-restic-server/internal/config"
	"github.com/labstack/echo/v5"
)

func TestBuildIPExtractor_XForwardedForOnlyFromTrustedProxies(t *testing.T) {
	tests := []struct {
		name string
		acl  *config.ACLConfig
		want string
	}{
		{"no acl", nil, "192.168.1.20"},
		{"acl without trusted proxies", &config.ACLConfig{}, "192.168.1.20"},
		{"peer is a trusted proxy", &config.ACLConfig{TrustedProxies: []string{"192.168.1.0/24"}}, "10.0.0.5"},
		{"peer is not a trusted proxy", &config.ACLConfig{TrustedProxies: []string{"172.16.0.0/12"}}, "192.168.1.20"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extract, err := buildIPExtractor(&config.Config{ListenMode: "plain", ACL: tt.acl})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodGet, "/config", nil)
			req.RemoteAddr = "192.168.1.20:40000"
			req.Header.Set(echo.HeaderXForwardedFor, "10.0.0.5")
			if got := extract(req); got != tt.want {
				t.Errorf("client IP = %q, want %q", got, tt.want)
			}
		})
	}
}
