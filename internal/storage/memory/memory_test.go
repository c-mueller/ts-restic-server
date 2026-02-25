package memory

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/c-mueller/ts-restic-server/internal/middleware"
	"github.com/c-mueller/ts-restic-server/internal/storage"
	"github.com/c-mueller/ts-restic-server/internal/storage/backendtest"
)

func TestSuite(t *testing.T) {
	backendtest.RunSuite(t, func(t *testing.T) storage.Backend {
		return New(10 * 1024 * 1024) // 10 MB
	})
}

func TestMemoryBackend_QuotaExceeded_SaveBlob(t *testing.T) {
	b := New(100) // 100 bytes quota
	ctx := middleware.ContextWithRepoPrefix(context.Background(), "quota-test")
	if err := b.CreateRepo(ctx); err != nil {
		t.Fatal(err)
	}

	err := b.SaveBlob(ctx, storage.BlobData, "aa00", bytes.NewReader(make([]byte, 200)))
	if !errors.Is(err, storage.ErrQuotaExceeded) {
		t.Fatalf("want ErrQuotaExceeded, got %v", err)
	}
}

func TestMemoryBackend_QuotaExceeded_SaveConfig(t *testing.T) {
	b := New(100)
	ctx := middleware.ContextWithRepoPrefix(context.Background(), "quota-cfg")
	if err := b.CreateRepo(ctx); err != nil {
		t.Fatal(err)
	}

	err := b.SaveConfig(ctx, bytes.NewReader(make([]byte, 200)))
	if !errors.Is(err, storage.ErrQuotaExceeded) {
		t.Fatalf("want ErrQuotaExceeded, got %v", err)
	}
}

func TestMemoryBackend_QuotaReclaimedOnDelete(t *testing.T) {
	b := New(100)
	ctx := middleware.ContextWithRepoPrefix(context.Background(), "quota-reclaim")
	if err := b.CreateRepo(ctx); err != nil {
		t.Fatal(err)
	}

	// Fill up quota
	data := make([]byte, 80)
	if err := b.SaveBlob(ctx, storage.BlobData, "aa01", bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}

	// This should exceed quota
	err := b.SaveBlob(ctx, storage.BlobData, "aa02", bytes.NewReader(make([]byte, 30)))
	if !errors.Is(err, storage.ErrQuotaExceeded) {
		t.Fatalf("want ErrQuotaExceeded, got %v", err)
	}

	// Delete the first blob to reclaim space
	if err := b.DeleteBlob(ctx, storage.BlobData, "aa01"); err != nil {
		t.Fatal(err)
	}

	// Now the same write should succeed
	if err := b.SaveBlob(ctx, storage.BlobData, "aa02", bytes.NewReader(make([]byte, 30))); err != nil {
		t.Fatalf("SaveBlob after reclaim: %v", err)
	}
}
