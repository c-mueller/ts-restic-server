// Package tsauth handles Tailscale node bootstrap: resolving the auth key from
// its possible sources, checking for persisted node state, and waiting for the
// node to come up.
package tsauth

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Source identifies where an auth key came from.
type Source int

const (
	SourceNone Source = iota
	SourceFlag
	SourceStdin
	SourceEnv
	SourceConfig
)

func (s Source) String() string {
	switch s {
	case SourceNone:
		return "none"
	case SourceFlag:
		return "flag"
	case SourceStdin:
		return "stdin"
	case SourceEnv:
		return "environment"
	case SourceConfig:
		return "config file"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}

// Key is a resolved auth key together with its origin.
type Key struct {
	Value  string
	Source Source
}

// EnvVars are the environment variables checked for an auth key, in order of
// precedence. TS_AUTHKEY and TS_AUTH_KEY follow the Tailscale convention;
// RESTIC_TAILSCALE_AUTH_KEY is the server's own config env mapping.
var EnvVars = []string{"TS_AUTHKEY", "TS_AUTH_KEY", "RESTIC_TAILSCALE_AUTH_KEY"}

var (
	ErrFlagAndStdin = errors.New("--auth-key and --auth-key-stdin are mutually exclusive")
	ErrEmptyStdin   = errors.New("no auth key received on stdin")
)

// Sources holds every place an auth key may be supplied from.
type Sources struct {
	// Flag is the value of --auth-key.
	Flag string
	// Stdin is non-nil when --auth-key-stdin was given.
	Stdin io.Reader
	// Getenv looks up environment variables (normally os.Getenv).
	Getenv func(string) string
	// Config is tailscale.auth_key from the loaded config, after ${VAR}
	// substitution.
	Config string
	// ConfigRaw is the same value before ${VAR} substitution.
	ConfigRaw string
}

// ResolveAuthKey picks the auth key by precedence: flag, stdin, environment,
// config file. A config value written as a ${VAR} placeholder counts as coming
// from the environment, since the key itself never sits in the file; an
// unresolved placeholder is ignored.
// Finding no key at all is not an error; Key.Source is then SourceNone.
func ResolveAuthKey(s Sources) (Key, error) {
	if s.Flag != "" && s.Stdin != nil {
		return Key{}, ErrFlagAndStdin
	}

	if v := strings.TrimSpace(s.Flag); v != "" {
		return Key{Value: v, Source: SourceFlag}, nil
	}

	if s.Stdin != nil {
		line, err := bufio.NewReader(s.Stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return Key{}, fmt.Errorf("reading auth key from stdin: %w", err)
		}
		v := strings.TrimSpace(line)
		if v == "" {
			return Key{}, ErrEmptyStdin
		}
		return Key{Value: v, Source: SourceStdin}, nil
	}

	if s.Getenv != nil {
		for _, name := range EnvVars {
			if v := strings.TrimSpace(s.Getenv(name)); v != "" {
				return Key{Value: v, Source: SourceEnv}, nil
			}
		}
	}

	// A value that still contains a placeholder was left unresolved by lenient
	// env substitution and is not a key.
	if v := strings.TrimSpace(s.Config); v != "" && !strings.Contains(v, "${") {
		if strings.Contains(s.ConfigRaw, "${") {
			return Key{Value: v, Source: SourceEnv}, nil
		}
		return Key{Value: v, Source: SourceConfig}, nil
	}

	return Key{Source: SourceNone}, nil
}
