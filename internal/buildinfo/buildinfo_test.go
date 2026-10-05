package buildinfo

import (
	"runtime/debug"
	"testing"
)

const sha = "def2039871cb6e90e5963efbf040d3ed0b34d04e" // pragma: allowlist secret

func vcsBuild(mainVersion string, settings ...debug.BuildSetting) *debug.BuildInfo {
	bi := &debug.BuildInfo{Settings: settings}
	bi.Main.Version = mainVersion
	return bi
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name                  string
		version, commit, date string
		bi                    *debug.BuildInfo
		want                  Info
	}{
		{
			name:    "goreleaser release",
			version: "0.0.6", commit: sha, date: "2026-10-05T00:30:00Z",
			want: Info{Version: "v0.0.6", Commit: sha, BuildDate: "2026-10-05T00:30:00Z", Channel: "stable"},
		},
		{
			name:    "goreleaser snapshot",
			version: "0.0.6-SNAPSHOT-def2039", commit: sha, date: "2026-10-05T00:30:00Z",
			want: Info{Version: "v0.0.6-SNAPSHOT-def2039", Commit: sha, BuildDate: "2026-10-05T00:30:00Z", Channel: "unstable"},
		},
		{
			name:    "docker tag build",
			version: "v0.0.6", commit: sha, date: "2026-10-05T00:30:00Z",
			want: Info{Version: "v0.0.6", Commit: sha, BuildDate: "2026-10-05T00:30:00Z", Channel: "stable"},
		},
		{
			name:    "docker master build",
			version: "v0.0.5-10-gdef2039", commit: sha, date: "2026-10-05T00:30:00Z",
			want: Info{Version: "v0.0.5-10-gdef2039", Commit: sha, BuildDate: "2026-10-05T00:30:00Z", Channel: "unstable"},
		},
		{
			name:    "pre-release tag",
			version: "v0.1.0-rc.1", commit: sha, date: "2026-10-05T00:30:00Z",
			want: Info{Version: "v0.1.0-rc.1", Commit: sha, BuildDate: "2026-10-05T00:30:00Z", Channel: "unstable"},
		},
		{
			name:    "plain go build between tags",
			version: "dev", commit: "unknown", date: "unknown",
			bi: vcsBuild("v0.0.6-0.20261005001100-def2039871cb+dirty",
				debug.BuildSetting{Key: "vcs.revision", Value: sha},
				debug.BuildSetting{Key: "vcs.modified", Value: "true"}),
			want: Info{Version: "v0.0.6-0.20261005001100-def2039871cb+dirty", Commit: sha, BuildDate: "unknown", Channel: "unstable"},
		},
		{
			name:    "plain go build on clean tag",
			version: "dev", commit: "unknown", date: "unknown",
			bi:   vcsBuild("v0.0.6", debug.BuildSetting{Key: "vcs.revision", Value: sha}),
			want: Info{Version: "v0.0.6", Commit: sha, BuildDate: "unknown", Channel: "stable"},
		},
		{
			name:    "ldflags win over vcs stamp",
			version: "0.0.6", commit: sha, date: "2026-10-05T00:30:00Z",
			bi:   vcsBuild("v0.0.5", debug.BuildSetting{Key: "vcs.revision", Value: "0000000000000000000000000000000000000000"}),
			want: Info{Version: "v0.0.6", Commit: sha, BuildDate: "2026-10-05T00:30:00Z", Channel: "stable"},
		},
		{
			name:    "build without vcs stamp",
			version: "dev", commit: "unknown", date: "unknown",
			bi:   vcsBuild("(devel)"),
			want: Info{Version: "dev", Commit: "unknown", BuildDate: "unknown", Channel: "unstable"},
		},
		{
			name:    "no build info at all",
			version: "", commit: "", date: "",
			want: Info{Version: "dev", Commit: "unknown", BuildDate: "unknown", Channel: "unstable"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolve(tt.version, tt.commit, tt.date, tt.bi)
			if got != tt.want {
				t.Errorf("resolve() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
