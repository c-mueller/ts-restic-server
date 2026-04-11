package buildinfo

// Set via -ldflags at build time by goreleaser or Docker builds.
// When built with plain "go build", these retain their defaults.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)
