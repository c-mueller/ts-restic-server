package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/c-mueller/ts-restic-server/internal/config"
	"github.com/c-mueller/ts-restic-server/internal/server"
	"github.com/c-mueller/ts-restic-server/internal/storage"
	"go.uber.org/zap"
)

const testPassword = "integration-test-password"
const testSeed int64 = 42

// requireRestic skips the test if the restic binary is not available.
func requireRestic(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("restic"); err != nil {
		t.Skip("restic binary not found, skipping integration test")
	}
}

// requireDocker skips the test if Docker is not available.
func requireDocker(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker not available, skipping integration test")
	}
}

// startServer starts a ts-restic-server instance with the given backend on a
// random port and returns its base URL. The server is stopped when the test ends.
func startServer(t *testing.T, backend storage.Backend) string {
	t.Helper()

	cfg := &config.Config{
		ListenMode: "plain",
		Storage:    config.Storage{Backend: "test"},
	}
	logger := zap.NewNop()
	srv := server.New(cfg, backend, logger, nil, nil)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	go srv.ServeOnListener(ln)
	t.Cleanup(func() { ln.Close() })

	return fmt.Sprintf("http://%s", ln.Addr().String())
}

// resticCmd runs a restic CLI command against the given repo URL and returns
// its combined output. The test fails immediately if the command exits non-zero.
func resticCmd(t *testing.T, repoURL, cacheDir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("restic", args...)
	cmd.Env = append(os.Environ(),
		"RESTIC_REPOSITORY=rest:"+repoURL,
		"RESTIC_PASSWORD="+testPassword,
		"RESTIC_CACHE_DIR="+cacheDir,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("restic %s failed: %v\noutput:\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// generateDatasetA creates the mixed-size test dataset (~100 MB):
//   - 50 × 1 MB, 100 × 100 KB, 200 × 10 KB (poorly compressible random data)
func generateDatasetA(t *testing.T, dir string) {
	t.Helper()
	rng := rand.New(rand.NewSource(testSeed))

	for i := 0; i < 50; i++ {
		writeRandomFile(t, filepath.Join(dir, "large", fmt.Sprintf("file_%03d.bin", i)), 1024*1024, rng)
	}
	for i := 0; i < 100; i++ {
		writeRandomFile(t, filepath.Join(dir, "medium", fmt.Sprintf("file_%03d.bin", i)), 100*1024, rng)
	}
	for i := 0; i < 200; i++ {
		writeRandomFile(t, filepath.Join(dir, "small", fmt.Sprintf("file_%03d.bin", i)), 10*1024, rng)
	}
}

// generateDatasetB creates a single 100 MB file (edge case for pack handling).
func generateDatasetB(t *testing.T, dir string) {
	t.Helper()
	rng := rand.New(rand.NewSource(testSeed + 100))
	writeRandomFile(t, filepath.Join(dir, "single_large.bin"), 100*1024*1024, rng)
}

// addDeltaFiles adds 10 × 512 KB files to an existing dataset directory.
func addDeltaFiles(t *testing.T, dir string) {
	t.Helper()
	rng := rand.New(rand.NewSource(testSeed + 1))
	for i := 0; i < 10; i++ {
		writeRandomFile(t, filepath.Join(dir, "delta", fmt.Sprintf("file_%03d.bin", i)), 512*1024, rng)
	}
}

// writeRandomFile writes a file filled with deterministic random bytes.
// Data is written in 64 KB chunks to avoid large allocations for big files.
func writeRandomFile(t *testing.T, path string, size int, rng *rand.Rand) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	defer f.Close()

	buf := make([]byte, 64*1024)
	remaining := size
	for remaining > 0 {
		n := len(buf)
		if n > remaining {
			n = remaining
		}
		rng.Read(buf[:n])
		if _, err := f.Write(buf[:n]); err != nil {
			t.Fatalf("write file: %v", err)
		}
		remaining -= n
	}
}

// hashDirectory computes a deterministic SHA-256 digest over all files in dir.
// Files are sorted by relative path; both path names and content contribute to
// the hash, so any change in file names, ordering, or data is detected.
func hashDirectory(t *testing.T, dir string) string {
	t.Helper()
	h := sha256.New()

	var entries []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			rel, _ := filepath.Rel(dir, path)
			entries = append(entries, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	sort.Strings(entries)

	for _, rel := range entries {
		fmt.Fprintf(h, "%s\n", rel)
		f, err := os.Open(filepath.Join(dir, rel))
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		if _, err := io.Copy(h, f); err != nil {
			f.Close()
			t.Fatalf("read: %v", err)
		}
		f.Close()
	}

	return hex.EncodeToString(h.Sum(nil))
}
