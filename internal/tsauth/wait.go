package tsauth

import (
	"errors"
	"fmt"

	"tailscale.com/ipn"
)

// ErrNeedsLogin is returned when the node requires an interactive browser
// login that was not allowed.
var ErrNeedsLogin = errors.New("tailscale node needs interactive login")

// WaitOptions controls how WaitRunning reacts to login-related states.
type WaitOptions struct {
	// AllowInteractive keeps waiting when the node asks for a browser login
	// instead of failing with ErrNeedsLogin.
	AllowInteractive bool
	// OnAuthURL, if set, is called with the login URL in interactive mode.
	OnAuthURL func(url string)
	// OnNeedsMachineAuth, if set, is called once when the node waits for
	// approval in the admin console.
	OnNeedsMachineAuth func()
}

// WaitRunning consumes IPN bus notifications from next until the backend is
// Running. A NeedsLogin state alone is not treated as failure, since tsnet
// passes through it briefly on every start; the definitive signal for a
// required interactive login is a login URL (BrowseToURL).
func WaitRunning(next func() (ipn.Notify, error), opts WaitOptions) error {
	machineAuthReported := false
	for {
		n, err := next()
		if err != nil {
			return fmt.Errorf("waiting for tailscale node: %w", err)
		}
		if n.ErrMessage != nil {
			return fmt.Errorf("tailscale backend: %s", *n.ErrMessage)
		}
		if n.BrowseToURL != nil && *n.BrowseToURL != "" {
			if !opts.AllowInteractive {
				return ErrNeedsLogin
			}
			if opts.OnAuthURL != nil {
				opts.OnAuthURL(*n.BrowseToURL)
			}
		}
		if n.State == nil {
			continue
		}
		switch *n.State {
		case ipn.Running:
			return nil
		case ipn.NeedsMachineAuth:
			if !machineAuthReported && opts.OnNeedsMachineAuth != nil {
				opts.OnNeedsMachineAuth()
			}
			machineAuthReported = true
		}
	}
}
