package instrumented

import (
	"context"
	"io"
	"time"

	"github.com/c-mueller/ts-restic-server/internal/metrics"
	"github.com/c-mueller/ts-restic-server/internal/storage"
)

// Backend wraps a storage.Backend and records Prometheus metrics for each operation.
type Backend struct {
	inner       storage.Backend
	backendName string
}

// New returns an instrumented wrapper around the given backend.
func New(inner storage.Backend, backendName string) *Backend {
	return &Backend{inner: inner, backendName: backendName}
}

func (b *Backend) observe(op string, start time.Time, err error) {
	duration := time.Since(start).Seconds()
	result := "success"
	if err != nil {
		result = "error"
	}
	metrics.StorageOperationDuration.WithLabelValues(op, b.backendName).Observe(duration)
	metrics.StorageOperationsTotal.WithLabelValues(op, b.backendName, result).Inc()
}

func (b *Backend) CreateRepo(ctx context.Context) error {
	start := time.Now()
	err := b.inner.CreateRepo(ctx)
	b.observe("CreateRepo", start, err)
	return err
}

func (b *Backend) DeleteRepo(ctx context.Context) error {
	start := time.Now()
	err := b.inner.DeleteRepo(ctx)
	b.observe("DeleteRepo", start, err)
	return err
}

func (b *Backend) StatConfig(ctx context.Context) (int64, error) {
	start := time.Now()
	size, err := b.inner.StatConfig(ctx)
	b.observe("StatConfig", start, err)
	return size, err
}

func (b *Backend) GetConfig(ctx context.Context) (io.ReadCloser, error) {
	start := time.Now()
	rc, err := b.inner.GetConfig(ctx)
	b.observe("GetConfig", start, err)
	return rc, err
}

func (b *Backend) SaveConfig(ctx context.Context, data io.Reader) error {
	start := time.Now()
	err := b.inner.SaveConfig(ctx, data)
	b.observe("SaveConfig", start, err)
	return err
}

func (b *Backend) StatBlob(ctx context.Context, t storage.BlobType, name string) (int64, error) {
	start := time.Now()
	size, err := b.inner.StatBlob(ctx, t, name)
	b.observe("StatBlob", start, err)
	return size, err
}

func (b *Backend) GetBlob(ctx context.Context, t storage.BlobType, name string, offset, length int64) (io.ReadCloser, error) {
	start := time.Now()
	rc, err := b.inner.GetBlob(ctx, t, name, offset, length)
	b.observe("GetBlob", start, err)
	return rc, err
}

func (b *Backend) SaveBlob(ctx context.Context, t storage.BlobType, name string, data io.Reader) error {
	start := time.Now()
	err := b.inner.SaveBlob(ctx, t, name, data)
	b.observe("SaveBlob", start, err)
	return err
}

func (b *Backend) DeleteBlob(ctx context.Context, t storage.BlobType, name string) error {
	start := time.Now()
	err := b.inner.DeleteBlob(ctx, t, name)
	b.observe("DeleteBlob", start, err)
	return err
}

func (b *Backend) ListBlobs(ctx context.Context, t storage.BlobType) ([]storage.Blob, error) {
	start := time.Now()
	blobs, err := b.inner.ListBlobs(ctx, t)
	b.observe("ListBlobs", start, err)
	return blobs, err
}
