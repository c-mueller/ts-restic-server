package tsauth

import (
	"errors"
	"strings"
	"testing"
)

// env returns a Getenv function backed by the given map.
func env(vars map[string]string) func(string) string {
	return func(name string) string { return vars[name] }
}

func TestResolveAuthKey_NoSource(t *testing.T) {
	key, err := ResolveAuthKey(Sources{Getenv: env(nil)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key.Value != "" || key.Source != SourceNone {
		t.Fatalf("got %+v, want empty key from SourceNone", key)
	}
}

func TestResolveAuthKey_Flag(t *testing.T) {
	key, err := ResolveAuthKey(Sources{Flag: "tskey-flag", Getenv: env(nil)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key.Value != "tskey-flag" || key.Source != SourceFlag {
		t.Fatalf("got %+v, want tskey-flag from SourceFlag", key)
	}
}

func TestResolveAuthKey_FlagBeatsEnvAndConfig(t *testing.T) {
	key, err := ResolveAuthKey(Sources{
		Flag:   "tskey-flag",
		Getenv: env(map[string]string{"TS_AUTHKEY": "tskey-env"}),
		Config: "tskey-config",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key.Value != "tskey-flag" || key.Source != SourceFlag {
		t.Fatalf("got %+v, want tskey-flag from SourceFlag", key)
	}
}

func TestResolveAuthKey_StdinTrimmed(t *testing.T) {
	key, err := ResolveAuthKey(Sources{Stdin: strings.NewReader("  tskey-stdin\n"), Getenv: env(nil)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key.Value != "tskey-stdin" || key.Source != SourceStdin {
		t.Fatalf("got %+v, want tskey-stdin from SourceStdin", key)
	}
}

func TestResolveAuthKey_StdinReadsFirstLineOnly(t *testing.T) {
	key, err := ResolveAuthKey(Sources{Stdin: strings.NewReader("tskey-first\nsecond-line\n"), Getenv: env(nil)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key.Value != "tskey-first" {
		t.Fatalf("got %q, want tskey-first", key.Value)
	}
}

func TestResolveAuthKey_StdinWithoutTrailingNewline(t *testing.T) {
	key, err := ResolveAuthKey(Sources{Stdin: strings.NewReader("tskey-nonl"), Getenv: env(nil)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key.Value != "tskey-nonl" {
		t.Fatalf("got %q, want tskey-nonl", key.Value)
	}
}

func TestResolveAuthKey_StdinBeatsEnvAndConfig(t *testing.T) {
	key, err := ResolveAuthKey(Sources{
		Stdin:  strings.NewReader("tskey-stdin\n"),
		Getenv: env(map[string]string{"TS_AUTHKEY": "tskey-env"}),
		Config: "tskey-config",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key.Source != SourceStdin {
		t.Fatalf("got source %v, want SourceStdin", key.Source)
	}
}

func TestResolveAuthKey_StdinEmpty(t *testing.T) {
	_, err := ResolveAuthKey(Sources{Stdin: strings.NewReader("\n"), Getenv: env(nil)})
	if !errors.Is(err, ErrEmptyStdin) {
		t.Fatalf("got %v, want ErrEmptyStdin", err)
	}
}

func TestResolveAuthKey_FlagAndStdinExclusive(t *testing.T) {
	_, err := ResolveAuthKey(Sources{
		Flag:   "tskey-flag",
		Stdin:  strings.NewReader("tskey-stdin\n"),
		Getenv: env(nil),
	})
	if !errors.Is(err, ErrFlagAndStdin) {
		t.Fatalf("got %v, want ErrFlagAndStdin", err)
	}
}

func TestResolveAuthKey_EnvPrecedence(t *testing.T) {
	tests := []struct {
		name string
		vars map[string]string
		want string
	}{
		{"TS_AUTHKEY first", map[string]string{"TS_AUTHKEY": "a", "TS_AUTH_KEY": "b", "RESTIC_TAILSCALE_AUTH_KEY": "c"}, "a"},
		{"TS_AUTH_KEY second", map[string]string{"TS_AUTH_KEY": "b", "RESTIC_TAILSCALE_AUTH_KEY": "c"}, "b"},
		{"RESTIC_TAILSCALE_AUTH_KEY third", map[string]string{"RESTIC_TAILSCALE_AUTH_KEY": "c"}, "c"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key, err := ResolveAuthKey(Sources{Getenv: env(tc.vars)})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if key.Value != tc.want || key.Source != SourceEnv {
				t.Fatalf("got %+v, want %q from SourceEnv", key, tc.want)
			}
		})
	}
}

func TestResolveAuthKey_EnvBeatsConfig(t *testing.T) {
	key, err := ResolveAuthKey(Sources{
		Getenv: env(map[string]string{"RESTIC_TAILSCALE_AUTH_KEY": "tskey-env"}),
		Config: "tskey-env", // viper also maps RESTIC_TAILSCALE_AUTH_KEY into the config value
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key.Source != SourceEnv {
		t.Fatalf("got source %v, want SourceEnv", key.Source)
	}
}

func TestResolveAuthKey_Config(t *testing.T) {
	key, err := ResolveAuthKey(Sources{Getenv: env(nil), Config: "tskey-config", ConfigRaw: "tskey-config"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key.Value != "tskey-config" || key.Source != SourceConfig {
		t.Fatalf("got %+v, want tskey-config from SourceConfig", key)
	}
}

func TestResolveAuthKey_ConfigPlaceholderCountsAsEnv(t *testing.T) {
	// auth_key: ${MY_KEY} in the config file is resolved from the environment
	// by the config loader; the key itself never sits in the file.
	key, err := ResolveAuthKey(Sources{Getenv: env(nil), Config: "tskey-resolved", ConfigRaw: "${MY_KEY}"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key.Value != "tskey-resolved" || key.Source != SourceEnv {
		t.Fatalf("got %+v, want tskey-resolved from SourceEnv", key)
	}
}

func TestResolveAuthKey_UnresolvedPlaceholderIgnored(t *testing.T) {
	// With lenient env substitution an unset ${MY_KEY} stays literal; it must
	// not be passed on as an auth key.
	key, err := ResolveAuthKey(Sources{Getenv: env(nil), Config: "${MY_KEY}", ConfigRaw: "${MY_KEY}"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key.Value != "" || key.Source != SourceNone {
		t.Fatalf("got %+v, want no key", key)
	}
}

func TestResolveAuthKey_TrimsWhitespace(t *testing.T) {
	key, err := ResolveAuthKey(Sources{Flag: "  tskey-flag \n", Getenv: env(nil)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key.Value != "tskey-flag" {
		t.Fatalf("got %q, want tskey-flag", key.Value)
	}
}

func TestSourceString(t *testing.T) {
	tests := map[Source]string{
		SourceNone:   "none",
		SourceFlag:   "flag",
		SourceStdin:  "stdin",
		SourceEnv:    "environment",
		SourceConfig: "config file",
	}
	for src, want := range tests {
		if got := src.String(); got != want {
			t.Errorf("Source(%d).String() = %q, want %q", int(src), got, want)
		}
	}
}
