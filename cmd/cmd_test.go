package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c-mueller/ts-restic-server/internal/tsauth"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// run executes the root command with args and returns its error. Flag values
// persist between cobra executions, so all flags are reset first.
func run(t *testing.T, stdin string, args ...string) error {
	t.Helper()
	for _, c := range append([]*cobra.Command{rootCmd}, rootCmd.Commands()...) {
		resetFlags(c.Flags())
		resetFlags(c.PersistentFlags())
	}
	// No auth key may leak in from the developer's environment.
	for _, name := range tsauth.EnvVars {
		t.Setenv(name, "")
	}
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetIn(strings.NewReader(stdin))
	rootCmd.SetArgs(args)
	t.Cleanup(func() { rootCmd.SetArgs(nil) })
	return rootCmd.Execute()
}

func resetFlags(fs *pflag.FlagSet) {
	fs.VisitAll(func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	})
}

// writeConfig writes a YAML config into a temp dir and returns its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func tailscaleConfig(t *testing.T, stateDir string) string {
	return writeConfig(t, "listen_mode: tailscale\n"+
		"tailscale:\n  hostname: test-node\n  state_dir: "+stateDir+"\n"+
		"metrics:\n  enabled: false\n"+
		"storage:\n  backend: memory\n")
}

func assertErrContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("got error %v, want one containing %q", err, want)
	}
}

func TestServe_MissingExplicitConfigFails(t *testing.T) {
	err := run(t, "", "serve", "--config", filepath.Join(t.TempDir(), "missing.yaml"))
	assertErrContains(t, err, "missing.yaml")
}

func TestServe_BrokenConfigFails(t *testing.T) {
	p := writeConfig(t, "listen_mode: [unclosed\n")
	err := run(t, "", "serve", "--config", p)
	assertErrContains(t, err, "reading config file")
}

func TestServe_TailscaleWithoutStateOrKeyFails(t *testing.T) {
	stateDir := t.TempDir()
	err := run(t, "", "serve", "--config", tailscaleConfig(t, stateDir))
	if !errors.Is(err, tsauth.ErrNoState) {
		t.Fatalf("got %v, want ErrNoState", err)
	}
	assertErrContains(t, err, stateDir)
}

func TestInit_RejectsPlainMode(t *testing.T) {
	p := writeConfig(t, "listen_mode: plain\nstorage:\n  backend: memory\n")
	err := run(t, "", "init", "--config", p, "--auth-key", "tskey-test")
	assertErrContains(t, err, "listen_mode")
}

func TestInit_FlagAndStdinExclusive(t *testing.T) {
	err := run(t, "tskey-stdin\n", "init", "--config", tailscaleConfig(t, t.TempDir()),
		"--auth-key", "tskey-flag", "--auth-key-stdin")
	if !errors.Is(err, tsauth.ErrFlagAndStdin) {
		t.Fatalf("got %v, want ErrFlagAndStdin", err)
	}
}

func TestInit_EmptyStdinFails(t *testing.T) {
	err := run(t, "\n", "init", "--config", tailscaleConfig(t, t.TempDir()), "--auth-key-stdin")
	if !errors.Is(err, tsauth.ErrEmptyStdin) {
		t.Fatalf("got %v, want ErrEmptyStdin", err)
	}
}

func TestInit_NoKeyAndNoStateFails(t *testing.T) {
	err := run(t, "", "init", "--config", tailscaleConfig(t, t.TempDir()))
	assertErrContains(t, err, "no auth key")
}

func TestInit_ForceRequiresKey(t *testing.T) {
	err := run(t, "", "init", "--config", tailscaleConfig(t, t.TempDir()), "--force")
	assertErrContains(t, err, "--force")
}

func TestInit_UnresolvedStorageVarsAreTolerated(t *testing.T) {
	// init only needs the tailscale section; unset storage secrets must not
	// stop it. With no key and no state it must fail for that reason instead.
	p := writeConfig(t, "listen_mode: tailscale\n"+
		"tailscale:\n  hostname: test-node\n  state_dir: "+t.TempDir()+"\n"+
		"storage:\n  backend: s3\n  s3:\n    bucket: b\n    secret_key: ${TRS_TEST_UNSET_SECRET}\n")
	err := run(t, "", "init", "--config", p)
	assertErrContains(t, err, "no auth key")
}
