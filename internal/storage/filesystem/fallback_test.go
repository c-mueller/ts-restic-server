package filesystem_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/c-mueller/ts-restic-server/internal/middleware"
	"github.com/c-mueller/ts-restic-server/internal/storage"
	"github.com/c-mueller/ts-restic-server/internal/storage/filesystem"
)

// TestUnshardedFallback verifies that data blobs stored in the flat (unsharded)
// layout (data/<name>) are still accessible after sharding was enabled.
// New writes always go to the sharded path (data/<prefix>/<name>), but reads,
// stats, and deletes must fall back to the unsharded path when the sharded
// path does not exist.
func TestUnshardedFallback(t *testing.T) {
	dir := t.TempDir()
	ctx := middleware.ContextWithRepoPrefix(context.Background(), "test-repo")

	b, err := filesystem.New(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := b.CreateRepo(ctx); err != nil {
		t.Fatal(err)
	}

	// Manually place a data blob in the flat (unsharded) location to simulate
	// data written before sharding was enabled.
	blobName := "aabbccdd11223344"
	blobData := []byte("unsharded blob content")
	unshardedPath := filepath.Join(dir, "test-repo", "data", blobName)
	if err := os.WriteFile(unshardedPath, blobData, 0o644); err != nil {
		t.Fatalf("write unsharded blob: %v", err)
	}

	// Verify the sharded path does NOT exist.
	shardedPath := filepath.Join(dir, "test-repo", "data", blobName[:2], blobName)
	if _, err := os.Stat(shardedPath); !os.IsNotExist(err) {
		t.Fatal("sharded path should not exist before fallback test")
	}

	t.Run("StatBlob_Fallback", func(t *testing.T) {
		size, err := b.StatBlob(ctx, storage.BlobData, blobName)
		if err != nil {
			t.Fatalf("StatBlob unsharded fallback: %v", err)
		}
		if size != int64(len(blobData)) {
			t.Fatalf("StatBlob size = %d, want %d", size, len(blobData))
		}
	})

	t.Run("GetBlob_Fallback", func(t *testing.T) {
		rc, err := b.GetBlob(ctx, storage.BlobData, blobName, 0, 0)
		if err != nil {
			t.Fatalf("GetBlob unsharded fallback: %v", err)
		}
		defer rc.Close()
		got, _ := io.ReadAll(rc)
		if !bytes.Equal(got, blobData) {
			t.Fatalf("GetBlob = %q, want %q", got, blobData)
		}
	})

	t.Run("GetBlob_Fallback_Range", func(t *testing.T) {
		rc, err := b.GetBlob(ctx, storage.BlobData, blobName, 10, 4)
		if err != nil {
			t.Fatalf("GetBlob range unsharded fallback: %v", err)
		}
		defer rc.Close()
		got, _ := io.ReadAll(rc)
		if !bytes.Equal(got, blobData[10:14]) {
			t.Fatalf("GetBlob range = %q, want %q", got, blobData[10:14])
		}
	})

	t.Run("SaveBlob_SkipsIfUnshardedExists", func(t *testing.T) {
		// SaveBlob should recognize the blob exists in the unsharded path
		// and skip the write (content-addressed dedup).
		if err := b.SaveBlob(ctx, storage.BlobData, blobName, bytes.NewReader(blobData)); err != nil {
			t.Fatalf("SaveBlob: %v", err)
		}
		// The sharded path should still not exist (write was skipped).
		if _, err := os.Stat(shardedPath); !os.IsNotExist(err) {
			t.Fatal("SaveBlob should have skipped write since blob exists in unsharded path")
		}
	})

	t.Run("ListBlobs_IncludesUnsharded", func(t *testing.T) {
		// Also save a blob via the normal (sharded) path.
		shardedName := "ff00112233445566"
		shardedData := []byte("sharded blob content")
		if err := b.SaveBlob(ctx, storage.BlobData, shardedName, bytes.NewReader(shardedData)); err != nil {
			t.Fatalf("SaveBlob sharded: %v", err)
		}

		blobs, err := b.ListBlobs(ctx, storage.BlobData)
		if err != nil {
			t.Fatalf("ListBlobs: %v", err)
		}

		names := make(map[string]bool)
		for _, bl := range blobs {
			names[bl.Name] = true
		}
		if !names[blobName] {
			t.Errorf("ListBlobs missing unsharded blob %q", blobName)
		}
		if !names[shardedName] {
			t.Errorf("ListBlobs missing sharded blob %q", shardedName)
		}
	})

	t.Run("DeleteBlob_Fallback", func(t *testing.T) {
		if err := b.DeleteBlob(ctx, storage.BlobData, blobName); err != nil {
			t.Fatalf("DeleteBlob unsharded fallback: %v", err)
		}
		// Verify the file is gone.
		if _, err := os.Stat(unshardedPath); !os.IsNotExist(err) {
			t.Fatal("unsharded blob should have been deleted")
		}
		// Stat should now return ErrNotFound.
		_, err := b.StatBlob(ctx, storage.BlobData, blobName)
		if err != storage.ErrNotFound {
			t.Fatalf("StatBlob after delete: want ErrNotFound, got %v", err)
		}
	})

	t.Run("NonDataBlobs_NoFallback", func(t *testing.T) {
		// Verify that non-data blob types don't attempt any fallback.
		_, err := b.StatBlob(ctx, storage.BlobKeys, "deadbeef00000000")
		if err != storage.ErrNotFound {
			t.Fatalf("StatBlob keys: want ErrNotFound, got %v", err)
		}
	})
}
