package integration

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/c-mueller/ts-restic-server/internal/storage"
)

type resticSnapshot struct {
	ShortID string `json:"short_id"`
	Time    string `json:"time"`
}

// listSnapshots returns all snapshots in the repo, sorted by time ascending.
func listSnapshots(t *testing.T, repoURL, cacheDir string) []resticSnapshot {
	t.Helper()
	out := resticCmd(t, repoURL, cacheDir, "snapshots", "--json")
	trimmed := strings.TrimSpace(out)
	if trimmed == "null" || trimmed == "" || trimmed == "[]" {
		return nil
	}
	var snaps []resticSnapshot
	if err := json.Unmarshal([]byte(trimmed), &snaps); err != nil {
		t.Fatalf("parse snapshots JSON: %v\nraw output:\n%s", err, out)
	}
	sort.Slice(snaps, func(i, j int) bool {
		return snaps[i].Time < snaps[j].Time
	})
	return snaps
}

// runBackendSuite exercises a storage.Backend through the full restic
// lifecycle: backup, list, restore with integrity check, forget + prune.
func runBackendSuite(t *testing.T, backend storage.Backend) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	requireRestic(t)

	baseURL := startServer(t, backend)

	t.Run("DatasetA", func(t *testing.T) {
		runDatasetATest(t, baseURL+"/dataset-a")
	})
	t.Run("DatasetB", func(t *testing.T) {
		runDatasetBTest(t, baseURL+"/dataset-b")
	})
}

// runDatasetATest is the full mixed-size lifecycle test:
//
//  1. Generate deterministic test data (~100 MB mixed sizes)
//  2. restic init
//  3. restic backup → Snapshot 1
//  4. Add delta files, restic backup → Snapshot 2
//  5. List snapshots (expect 2)
//  6. Restore Snapshot 1, verify hash
//  7. Restore Snapshot 2, verify hash
//  8. Forget Snapshot 1 + prune
//  9. Verify only Snapshot 2 remains
func runDatasetATest(t *testing.T, repoURL string) {
	cacheDir := t.TempDir()

	// Create source data
	sourceDir := t.TempDir()
	sourceDir, _ = filepath.EvalSymlinks(sourceDir)
	generateDatasetA(t, sourceDir)
	originalHash := hashDirectory(t, sourceDir)

	// Init
	resticCmd(t, repoURL, cacheDir, "init")

	// Backup 1
	resticCmd(t, repoURL, cacheDir, "backup", sourceDir)

	// Add delta files, backup 2
	addDeltaFiles(t, sourceDir)
	deltaHash := hashDirectory(t, sourceDir)
	resticCmd(t, repoURL, cacheDir, "backup", sourceDir)

	// List snapshots — expect exactly 2
	snapshots := listSnapshots(t, repoURL, cacheDir)
	if len(snapshots) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snapshots))
	}

	// Restore snapshot 1 and verify integrity
	restoreDir1 := t.TempDir()
	restoreDir1, _ = filepath.EvalSymlinks(restoreDir1)
	resticCmd(t, repoURL, cacheDir, "restore", snapshots[0].ShortID, "--target", restoreDir1)
	restoredHash1 := hashDirectory(t, filepath.Join(restoreDir1, sourceDir))
	if restoredHash1 != originalHash {
		t.Fatal("snapshot 1 restore: hash mismatch — data integrity violation")
	}

	// Restore snapshot 2 and verify integrity
	restoreDir2 := t.TempDir()
	restoreDir2, _ = filepath.EvalSymlinks(restoreDir2)
	resticCmd(t, repoURL, cacheDir, "restore", snapshots[1].ShortID, "--target", restoreDir2)
	restoredHash2 := hashDirectory(t, filepath.Join(restoreDir2, sourceDir))
	if restoredHash2 != deltaHash {
		t.Fatal("snapshot 2 restore: hash mismatch — data integrity violation")
	}

	// Forget snapshot 1 + prune
	resticCmd(t, repoURL, cacheDir, "forget", snapshots[0].ShortID, "--prune")

	// Verify only snapshot 2 remains
	remaining := listSnapshots(t, repoURL, cacheDir)
	if len(remaining) != 1 {
		t.Fatalf("expected 1 snapshot after forget, got %d", len(remaining))
	}
	if remaining[0].ShortID != snapshots[1].ShortID {
		t.Fatal("wrong snapshot remaining after forget + prune")
	}
}

// runDatasetBTest exercises the single-large-file edge case (100 MB):
// init → backup → restore → verify hash.
func runDatasetBTest(t *testing.T, repoURL string) {
	cacheDir := t.TempDir()

	sourceDir := t.TempDir()
	sourceDir, _ = filepath.EvalSymlinks(sourceDir)
	generateDatasetB(t, sourceDir)
	originalHash := hashDirectory(t, sourceDir)

	resticCmd(t, repoURL, cacheDir, "init")
	resticCmd(t, repoURL, cacheDir, "backup", sourceDir)

	snapshots := listSnapshots(t, repoURL, cacheDir)
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}

	restoreDir := t.TempDir()
	restoreDir, _ = filepath.EvalSymlinks(restoreDir)
	resticCmd(t, repoURL, cacheDir, "restore", snapshots[0].ShortID, "--target", restoreDir)

	restoredHash := hashDirectory(t, filepath.Join(restoreDir, sourceDir))
	if restoredHash != originalHash {
		t.Fatal("dataset B restore: hash mismatch — data integrity violation")
	}
}
