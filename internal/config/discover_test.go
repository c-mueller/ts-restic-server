package config

import (
	"os"
	"path/filepath"
	"testing"
)

// touch creates an empty file at dir/name and returns its path.
func touch(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("listen: \":8880\"\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

func TestFindConfigFile_ExplicitExisting(t *testing.T) {
	dir := t.TempDir()
	p := touch(t, dir, "custom.yaml")

	got, err := FindConfigFile(p, []string{t.TempDir()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != p {
		t.Fatalf("got %q, want %q", got, p)
	}
}

func TestFindConfigFile_ExplicitMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.yaml")

	if _, err := FindConfigFile(missing, nil); err == nil {
		t.Fatal("expected error for missing explicit config file")
	}
}

func TestFindConfigFile_ExplicitIsDirectory(t *testing.T) {
	if _, err := FindConfigFile(t.TempDir(), nil); err == nil {
		t.Fatal("expected error when explicit config path is a directory")
	}
}

func TestFindConfigFile_FirstSearchDirWins(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	want := touch(t, first, "config.yaml")
	touch(t, second, "config.yaml")

	got, err := FindConfigFile("", []string{first, second})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFindConfigFile_FallsBackToLaterDir(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	want := touch(t, second, "config.yaml")

	got, err := FindConfigFile("", []string{first, second})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFindConfigFile_YamlPreferredOverYml(t *testing.T) {
	dir := t.TempDir()
	want := touch(t, dir, "config.yaml")
	touch(t, dir, "config.yml")

	got, err := FindConfigFile("", []string{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFindConfigFile_YmlFound(t *testing.T) {
	dir := t.TempDir()
	want := touch(t, dir, "config.yml")

	got, err := FindConfigFile("", []string{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFindConfigFile_IgnoresDirectoryNamedConfig(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(first, "config.yaml"), 0o700); err != nil {
		t.Fatal(err)
	}
	want := touch(t, second, "config.yml")

	got, err := FindConfigFile("", []string{first, second})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFindConfigFile_NothingFound(t *testing.T) {
	got, err := FindConfigFile("", []string{t.TempDir(), t.TempDir()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty path", got)
	}
}

func TestDefaultSearchDirs(t *testing.T) {
	want := []string{".", "/etc/ts-restic-server"}
	if len(DefaultSearchDirs) != len(want) {
		t.Fatalf("DefaultSearchDirs = %v, want %v", DefaultSearchDirs, want)
	}
	for i := range want {
		if DefaultSearchDirs[i] != want[i] {
			t.Fatalf("DefaultSearchDirs = %v, want %v", DefaultSearchDirs, want)
		}
	}
}
