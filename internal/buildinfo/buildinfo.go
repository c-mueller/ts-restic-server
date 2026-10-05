package buildinfo

import (
	"regexp"
	"runtime/debug"
	"strings"
)

// Set via -ldflags at build time by goreleaser or Docker builds.
// When built with plain "go build", these retain their defaults and Get
// falls back to the VCS stamp the Go toolchain embeds into the binary.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

const (
	ChannelStable   = "stable"
	ChannelUnstable = "unstable"
)

// Info describes the running binary.
type Info struct {
	// Version is the release tag (e.g. v0.0.6) or, for development
	// builds, the most specific version known (git describe, Go
	// pseudo-version, goreleaser snapshot) or "dev".
	Version string
	// Commit is the full git revision, also for release builds.
	Commit string
	// BuildDate is the RFC 3339 build time, "unknown" if not stamped.
	BuildDate string
	// Channel is ChannelStable for release tags, ChannelUnstable otherwise.
	Channel string
}

// releaseVersion matches plain release tags; anything with a pre-release,
// snapshot, describe or dirty suffix is a development build.
var releaseVersion = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// Get resolves the build information of the running binary.
func Get() Info {
	bi, _ := debug.ReadBuildInfo()
	return resolve(Version, Commit, BuildDate, bi)
}

func resolve(version, commit, buildDate string, bi *debug.BuildInfo) Info {
	if isUnset(version) && bi != nil && bi.Main.Version != "(devel)" {
		version = bi.Main.Version
	}
	if isUnset(commit) && bi != nil {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" {
				commit = s.Value
			}
		}
	}

	info := Info{
		Version:   orDefault(version, "dev"),
		Commit:    orDefault(commit, "unknown"),
		BuildDate: orDefault(buildDate, "unknown"),
		Channel:   ChannelUnstable,
	}
	// goreleaser passes the tag without its "v" prefix.
	if info.Version[0] >= '0' && info.Version[0] <= '9' {
		info.Version = "v" + info.Version
	}
	if releaseVersion.MatchString(info.Version) {
		info.Channel = ChannelStable
	}
	return info
}

func isUnset(v string) bool {
	v = strings.TrimSpace(v)
	return v == "" || v == "dev" || v == "unknown"
}

func orDefault(v, def string) string {
	if isUnset(v) {
		return def
	}
	return strings.TrimSpace(v)
}
