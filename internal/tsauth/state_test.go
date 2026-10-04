package tsauth

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStateExists_MissingDir(t *testing.T) {
	if StateExists(filepath.Join(t.TempDir(), "does-not-exist")) {
		t.Fatal("expected false for missing state dir")
	}
}

func TestStateExists_EmptyDir(t *testing.T) {
	if StateExists(t.TempDir()) {
		t.Fatal("expected false for empty state dir")
	}
}

func TestStateExists_EmptyStateFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, StateFileName), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if StateExists(dir) {
		t.Fatal("expected false for empty state file")
	}
}

func TestStateExists_StateFileIsDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, StateFileName), 0o700); err != nil {
		t.Fatal(err)
	}
	if StateExists(dir) {
		t.Fatal("expected false when state file is a directory")
	}
}

func TestStateExists_NonEmptyStateFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, StateFileName), []byte(`{"_profiles":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if !StateExists(dir) {
		t.Fatal("expected true for non-empty state file")
	}
}

func TestCheckServeStartup(t *testing.T) {
	withKey := Key{Value: "tskey", Source: SourceEnv}
	noKey := Key{Source: SourceNone}

	tests := []struct {
		name        string
		stateExists bool
		key         Key
		interactive bool
		wantErr     bool
	}{
		{"state present", true, noKey, false, false},
		{"state present with key", true, withKey, false, false},
		{"no state, key available", false, withKey, false, false},
		{"no state, interactive login allowed", false, noKey, true, false},
		{"no state, no key, not interactive", false, noKey, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckServeStartup("/var/lib/x/ts-state", tc.stateExists, tc.key, tc.interactive)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestCheckServeStartup_ErrorMessage(t *testing.T) {
	err := CheckServeStartup("/var/lib/x/ts-state", false, Key{}, false)
	if !errors.Is(err, ErrNoState) {
		t.Fatalf("got %v, want ErrNoState", err)
	}
	for _, want := range []string{"/var/lib/x/ts-state", "ts-restic-server init"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err.Error(), want)
		}
	}
}

func TestServeWarnings(t *testing.T) {
	configKey := Key{Value: "tskey", Source: SourceConfig}
	envKey := Key{Value: "tskey", Source: SourceEnv}

	tests := []struct {
		name        string
		stateExists bool
		key         Key
		wantSubstr  string // empty = expect no warning
	}{
		{"config key, state exists", true, configKey, "no longer needed"},
		{"config key, no state", false, configKey, "not recommended"},
		{"env key, no state", false, envKey, ""},
		{"env key, state exists", true, envKey, ""},
		{"no key", true, Key{}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ServeWarnings(tc.stateExists, tc.key)
			if tc.wantSubstr == "" {
				if len(got) != 0 {
					t.Fatalf("expected no warnings, got %v", got)
				}
				return
			}
			if len(got) != 1 || !strings.Contains(got[0], tc.wantSubstr) {
				t.Fatalf("got %v, want one warning containing %q", got, tc.wantSubstr)
			}
		})
	}
}
