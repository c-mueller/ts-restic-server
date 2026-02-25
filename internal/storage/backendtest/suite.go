package backendtest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sort"
	"testing"

	"github.com/c-mueller/ts-restic-server/internal/middleware"
	"github.com/c-mueller/ts-restic-server/internal/storage"
)

// RunSuite exercises every method of storage.Backend. newBackend is called per
// sub-test so each starts with a clean state.
func RunSuite(t *testing.T, newBackend func(t *testing.T) storage.Backend) {
	t.Helper()

	ctx := middleware.ContextWithRepoPrefix(context.Background(), "test-repo")

	t.Run("CreateRepo", func(t *testing.T) {
		b := newBackend(t)
		if err := b.CreateRepo(ctx); err != nil {
			t.Fatalf("CreateRepo: %v", err)
		}
		// Second call may return ErrRepoExists (memory) or nil (idempotent backends).
		err := b.CreateRepo(ctx)
		if err != nil && !errors.Is(err, storage.ErrRepoExists) {
			t.Fatalf("second CreateRepo: want nil or ErrRepoExists, got %v", err)
		}
	})

	t.Run("DeleteRepo", func(t *testing.T) {
		b := newBackend(t)
		if err := b.CreateRepo(ctx); err != nil {
			t.Fatalf("CreateRepo: %v", err)
		}
		if err := b.DeleteRepo(ctx); err != nil {
			t.Fatalf("DeleteRepo: %v", err)
		}
		// Second call may return ErrRepoNotFound (memory, filesystem) or nil (idempotent backends).
		err := b.DeleteRepo(ctx)
		if err != nil && !errors.Is(err, storage.ErrRepoNotFound) {
			t.Fatalf("DeleteRepo non-existent: want nil or ErrRepoNotFound, got %v", err)
		}
	})

	t.Run("StatConfig_NotFound", func(t *testing.T) {
		b := newBackend(t)
		if err := b.CreateRepo(ctx); err != nil {
			t.Fatalf("CreateRepo: %v", err)
		}
		_, err := b.StatConfig(ctx)
		if !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("StatConfig before save: want ErrNotFound, got %v", err)
		}
	})

	t.Run("SaveAndGetConfig", func(t *testing.T) {
		b := newBackend(t)
		if err := b.CreateRepo(ctx); err != nil {
			t.Fatalf("CreateRepo: %v", err)
		}

		data := []byte(`{"version":2}`)
		if err := b.SaveConfig(ctx, bytes.NewReader(data)); err != nil {
			t.Fatalf("SaveConfig: %v", err)
		}

		size, err := b.StatConfig(ctx)
		if err != nil {
			t.Fatalf("StatConfig: %v", err)
		}
		if size != int64(len(data)) {
			t.Fatalf("StatConfig size = %d, want %d", size, len(data))
		}

		rc, err := b.GetConfig(ctx)
		if err != nil {
			t.Fatalf("GetConfig: %v", err)
		}
		defer rc.Close()
		got, _ := io.ReadAll(rc)
		if !bytes.Equal(got, data) {
			t.Fatalf("GetConfig = %q, want %q", got, data)
		}
	})

	t.Run("SaveConfig_Overwrite", func(t *testing.T) {
		b := newBackend(t)
		if err := b.CreateRepo(ctx); err != nil {
			t.Fatalf("CreateRepo: %v", err)
		}

		first := []byte("first")
		second := []byte("second-longer")

		if err := b.SaveConfig(ctx, bytes.NewReader(first)); err != nil {
			t.Fatalf("first SaveConfig: %v", err)
		}
		if err := b.SaveConfig(ctx, bytes.NewReader(second)); err != nil {
			t.Fatalf("second SaveConfig: %v", err)
		}

		rc, err := b.GetConfig(ctx)
		if err != nil {
			t.Fatalf("GetConfig: %v", err)
		}
		defer rc.Close()
		got, _ := io.ReadAll(rc)
		if !bytes.Equal(got, second) {
			t.Fatalf("GetConfig after overwrite = %q, want %q", got, second)
		}
	})

	t.Run("StatBlob_NotFound", func(t *testing.T) {
		b := newBackend(t)
		if err := b.CreateRepo(ctx); err != nil {
			t.Fatalf("CreateRepo: %v", err)
		}
		_, err := b.StatBlob(ctx, storage.BlobData, "deadbeef")
		if !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("StatBlob missing: want ErrNotFound, got %v", err)
		}
	})

	t.Run("SaveAndGetBlob", func(t *testing.T) {
		b := newBackend(t)
		if err := b.CreateRepo(ctx); err != nil {
			t.Fatalf("CreateRepo: %v", err)
		}

		data := []byte("hello blob world")
		name := "aabbccdd"
		if err := b.SaveBlob(ctx, storage.BlobData, name, bytes.NewReader(data)); err != nil {
			t.Fatalf("SaveBlob: %v", err)
		}

		size, err := b.StatBlob(ctx, storage.BlobData, name)
		if err != nil {
			t.Fatalf("StatBlob: %v", err)
		}
		if size != int64(len(data)) {
			t.Fatalf("StatBlob size = %d, want %d", size, len(data))
		}

		rc, err := b.GetBlob(ctx, storage.BlobData, name, 0, 0)
		if err != nil {
			t.Fatalf("GetBlob: %v", err)
		}
		defer rc.Close()
		got, _ := io.ReadAll(rc)
		if !bytes.Equal(got, data) {
			t.Fatalf("GetBlob = %q, want %q", got, data)
		}
	})

	t.Run("GetBlob_Range", func(t *testing.T) {
		b := newBackend(t)
		if err := b.CreateRepo(ctx); err != nil {
			t.Fatalf("CreateRepo: %v", err)
		}

		data := []byte("0123456789abcdef")
		name := "aabb0001"
		if err := b.SaveBlob(ctx, storage.BlobData, name, bytes.NewReader(data)); err != nil {
			t.Fatalf("SaveBlob: %v", err)
		}

		rc, err := b.GetBlob(ctx, storage.BlobData, name, 4, 6)
		if err != nil {
			t.Fatalf("GetBlob range: %v", err)
		}
		defer rc.Close()
		got, _ := io.ReadAll(rc)
		if !bytes.Equal(got, data[4:10]) {
			t.Fatalf("GetBlob range = %q, want %q", got, data[4:10])
		}
	})

	t.Run("GetBlob_OffsetToEnd", func(t *testing.T) {
		b := newBackend(t)
		if err := b.CreateRepo(ctx); err != nil {
			t.Fatalf("CreateRepo: %v", err)
		}

		data := []byte("0123456789abcdef")
		name := "aabb0002"
		if err := b.SaveBlob(ctx, storage.BlobData, name, bytes.NewReader(data)); err != nil {
			t.Fatalf("SaveBlob: %v", err)
		}

		rc, err := b.GetBlob(ctx, storage.BlobData, name, 10, 0)
		if err != nil {
			t.Fatalf("GetBlob offset-to-end: %v", err)
		}
		defer rc.Close()
		got, _ := io.ReadAll(rc)
		if !bytes.Equal(got, data[10:]) {
			t.Fatalf("GetBlob offset-to-end = %q, want %q", got, data[10:])
		}
	})

	t.Run("DeleteBlob", func(t *testing.T) {
		b := newBackend(t)
		if err := b.CreateRepo(ctx); err != nil {
			t.Fatalf("CreateRepo: %v", err)
		}

		name := "aabb0003"
		if err := b.SaveBlob(ctx, storage.BlobData, name, bytes.NewReader([]byte("data"))); err != nil {
			t.Fatalf("SaveBlob: %v", err)
		}
		if err := b.DeleteBlob(ctx, storage.BlobData, name); err != nil {
			t.Fatalf("DeleteBlob: %v", err)
		}
		_, err := b.StatBlob(ctx, storage.BlobData, name)
		if !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("StatBlob after delete: want ErrNotFound, got %v", err)
		}
	})

	t.Run("DeleteBlob_NotFound", func(t *testing.T) {
		b := newBackend(t)
		if err := b.CreateRepo(ctx); err != nil {
			t.Fatalf("CreateRepo: %v", err)
		}
		// Use a valid hex name so filesystem shard directories resolve correctly.
		// Some backends (WebDAV) may return nil instead of ErrNotFound.
		err := b.DeleteBlob(ctx, storage.BlobKeys, "deadbeef00000000")
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("DeleteBlob non-existent: want nil or ErrNotFound, got %v", err)
		}
	})

	t.Run("ListBlobs_Empty", func(t *testing.T) {
		b := newBackend(t)
		if err := b.CreateRepo(ctx); err != nil {
			t.Fatalf("CreateRepo: %v", err)
		}
		blobs, err := b.ListBlobs(ctx, storage.BlobData)
		if err != nil {
			t.Fatalf("ListBlobs: %v", err)
		}
		if blobs == nil {
			t.Fatal("ListBlobs returned nil, want empty slice")
		}
		if len(blobs) != 0 {
			t.Fatalf("ListBlobs len = %d, want 0", len(blobs))
		}
	})

	t.Run("ListBlobs_WithData", func(t *testing.T) {
		b := newBackend(t)
		if err := b.CreateRepo(ctx); err != nil {
			t.Fatalf("CreateRepo: %v", err)
		}

		items := map[string][]byte{
			"aa01": []byte("one"),
			"bb02": []byte("twotwo"),
			"cc03": []byte("threethreethree"),
		}
		for name, data := range items {
			if err := b.SaveBlob(ctx, storage.BlobKeys, name, bytes.NewReader(data)); err != nil {
				t.Fatalf("SaveBlob %s: %v", name, err)
			}
		}

		blobs, err := b.ListBlobs(ctx, storage.BlobKeys)
		if err != nil {
			t.Fatalf("ListBlobs: %v", err)
		}
		if len(blobs) != 3 {
			t.Fatalf("ListBlobs len = %d, want 3", len(blobs))
		}

		sort.Slice(blobs, func(i, j int) bool { return blobs[i].Name < blobs[j].Name })
		for _, blob := range blobs {
			want := int64(len(items[blob.Name]))
			if blob.Size != want {
				t.Errorf("blob %s size = %d, want %d", blob.Name, blob.Size, want)
			}
		}
	})

	t.Run("AllBlobTypes", func(t *testing.T) {
		b := newBackend(t)
		if err := b.CreateRepo(ctx); err != nil {
			t.Fatalf("CreateRepo: %v", err)
		}

		types := []storage.BlobType{
			storage.BlobData, storage.BlobKeys, storage.BlobLocks,
			storage.BlobSnapshots, storage.BlobIndex,
		}

		for _, bt := range types {
			t.Run(string(bt), func(t *testing.T) {
				name := "ff00ff00"
				data := []byte("type-" + string(bt))

				if err := b.SaveBlob(ctx, bt, name, bytes.NewReader(data)); err != nil {
					t.Fatalf("SaveBlob: %v", err)
				}

				size, err := b.StatBlob(ctx, bt, name)
				if err != nil {
					t.Fatalf("StatBlob: %v", err)
				}
				if size != int64(len(data)) {
					t.Fatalf("size = %d, want %d", size, len(data))
				}

				rc, err := b.GetBlob(ctx, bt, name, 0, 0)
				if err != nil {
					t.Fatalf("GetBlob: %v", err)
				}
				got, _ := io.ReadAll(rc)
				rc.Close()
				if !bytes.Equal(got, data) {
					t.Fatalf("GetBlob = %q, want %q", got, data)
				}

				blobs, err := b.ListBlobs(ctx, bt)
				if err != nil {
					t.Fatalf("ListBlobs: %v", err)
				}
				if len(blobs) == 0 {
					t.Fatal("ListBlobs returned 0 blobs, want >= 1")
				}

				if err := b.DeleteBlob(ctx, bt, name); err != nil {
					t.Fatalf("DeleteBlob: %v", err)
				}
				_, err = b.StatBlob(ctx, bt, name)
				if !errors.Is(err, storage.ErrNotFound) {
					t.Fatalf("StatBlob after delete: want ErrNotFound, got %v", err)
				}
			})
		}
	})
}
