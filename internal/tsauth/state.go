package tsauth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// StateFileName is the file tsnet persists the node identity to inside its
// state directory.
const StateFileName = "tailscaled.state"

// ErrNoState is returned when serve would need to log in but has neither
// persisted state nor an auth key, and interactive login is not allowed.
var ErrNoState = errors.New("no Tailscale node state")

// StateExists reports whether dir holds a non-empty tsnet state file. It does
// not verify that the identity is still valid on the control server.
func StateExists(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, StateFileName))
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

// CheckServeStartup decides whether serve may start the Tailscale listener.
// Without persisted state, serve needs either an auth key or explicitly
// allowed interactive login; otherwise it fails instead of blocking on a
// login URL.
func CheckServeStartup(stateDir string, stateExists bool, key Key, interactive bool) error {
	if stateExists || key.Value != "" || interactive {
		return nil
	}
	return fmt.Errorf("%w in %s: run `ts-restic-server init` first (or allow interactive login with --tailscale-interactive-login)", ErrNoState, stateDir)
}

// ServeWarnings returns operator hints about how serve obtained its auth key.
// A key stored in the config file still works but is no longer recommended.
func ServeWarnings(stateExists bool, key Key) []string {
	if key.Source != SourceConfig {
		return nil
	}
	if stateExists {
		return []string{"tailscale.auth_key in the config file is no longer needed because node state exists; remove it from the config"}
	}
	return []string{"registering with tailscale.auth_key from the config file; this still works but is not recommended, use `ts-restic-server init` instead"}
}
