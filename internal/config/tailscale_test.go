package config

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestLoad_InteractiveLoginDefaultFalse(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	cfg, err := Load(false)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Tailscale.InteractiveLogin {
		t.Fatal("tailscale.interactive_login should default to false")
	}
}

func TestLoad_InteractiveLoginFromConfig(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("tailscale.interactive_login", true)

	cfg, err := Load(false)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Tailscale.InteractiveLogin {
		t.Fatal("tailscale.interactive_login: true was not loaded")
	}
}

func TestValidateForInit(t *testing.T) {
	valid := func() Config {
		c := validConfig()
		c.ListenMode = "tailscale"
		c.Tailscale = Tailscale{Hostname: "restic-server", StateDir: "/var/lib/ts-restic-server/ts-state"}
		return c
	}

	tests := []struct {
		name       string
		mutate     func(c *Config)
		wantSubstr string // empty = expect no error
	}{
		{"valid", func(c *Config) {}, ""},
		{"plain mode", func(c *Config) { c.ListenMode = "plain" }, "listen_mode"},
		{"empty hostname", func(c *Config) { c.Tailscale.Hostname = "" }, "tailscale.hostname"},
		{"empty state dir", func(c *Config) { c.Tailscale.StateDir = "" }, "tailscale.state_dir"},
		{"unresolved hostname", func(c *Config) { c.Tailscale.Hostname = "${TS_HOST}" }, "tailscale.hostname"},
		{"unresolved state dir", func(c *Config) { c.Tailscale.StateDir = "${TS_STATE}" }, "tailscale.state_dir"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := valid()
			tc.mutate(&c)
			err := c.ValidateForInit()
			if tc.wantSubstr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("got %v, want error mentioning %q", err, tc.wantSubstr)
			}
		})
	}
}
