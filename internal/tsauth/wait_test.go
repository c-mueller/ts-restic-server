package tsauth

import (
	"errors"
	"strings"
	"testing"

	"tailscale.com/ipn"
)

// feed returns a next function that yields the given notifications in order
// and then fails, so a wait loop that never returns is caught by the test.
func feed(notes ...ipn.Notify) func() (ipn.Notify, error) {
	i := 0
	return func() (ipn.Notify, error) {
		if i >= len(notes) {
			return ipn.Notify{}, errors.New("feed exhausted")
		}
		n := notes[i]
		i++
		return n, nil
	}
}

func state(s ipn.State) ipn.Notify { return ipn.Notify{State: &s} }

func authURL(u string) ipn.Notify { return ipn.Notify{BrowseToURL: &u} }

func TestWaitRunning_ReachesRunning(t *testing.T) {
	err := WaitRunning(feed(state(ipn.NoState), state(ipn.Starting), state(ipn.Running)), WaitOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWaitRunning_TransientNeedsLoginIsIgnored(t *testing.T) {
	// tsnet briefly reports NeedsLogin right after start even with valid state.
	err := WaitRunning(feed(state(ipn.NeedsLogin), state(ipn.Starting), state(ipn.Running)), WaitOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWaitRunning_AuthURLFailsWithoutInteractive(t *testing.T) {
	err := WaitRunning(feed(state(ipn.NeedsLogin), authURL("https://login.tailscale.com/a/abc")), WaitOptions{})
	if !errors.Is(err, ErrNeedsLogin) {
		t.Fatalf("got %v, want ErrNeedsLogin", err)
	}
}

func TestWaitRunning_AuthURLAllowedWhenInteractive(t *testing.T) {
	var seen string
	err := WaitRunning(
		feed(state(ipn.NeedsLogin), authURL("https://login.tailscale.com/a/abc"), state(ipn.Running)),
		WaitOptions{AllowInteractive: true, OnAuthURL: func(u string) { seen = u }},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seen != "https://login.tailscale.com/a/abc" {
		t.Fatalf("OnAuthURL got %q", seen)
	}
}

func TestWaitRunning_EmptyAuthURLIgnored(t *testing.T) {
	err := WaitRunning(feed(authURL(""), state(ipn.Running)), WaitOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWaitRunning_BackendErrorMessage(t *testing.T) {
	msg := "invalid key: unable to validate API key"
	err := WaitRunning(feed(state(ipn.Starting), ipn.Notify{ErrMessage: &msg}), WaitOptions{})
	if err == nil || !strings.Contains(err.Error(), msg) {
		t.Fatalf("got %v, want error containing %q", err, msg)
	}
}

func TestWaitRunning_NeedsMachineAuthCallbackOnce(t *testing.T) {
	calls := 0
	err := WaitRunning(
		feed(state(ipn.NeedsMachineAuth), state(ipn.NeedsMachineAuth), state(ipn.Running)),
		WaitOptions{OnNeedsMachineAuth: func() { calls++ }},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("OnNeedsMachineAuth called %d times, want 1", calls)
	}
}

func TestWaitRunning_NextError(t *testing.T) {
	boom := errors.New("watch closed")
	err := WaitRunning(func() (ipn.Notify, error) { return ipn.Notify{}, boom }, WaitOptions{})
	if !errors.Is(err, boom) {
		t.Fatalf("got %v, want wrapped %v", err, boom)
	}
}
