package tracked

import (
	"context"
	"io"

	"github.com/c-mueller/ts-restic-server/internal/middleware"
	"github.com/c-mueller/ts-restic-server/internal/stats"
	"github.com/c-mueller/ts-restic-server/internal/storage"
)

// Backend wraps a storage.Backend and records per-repository traffic
// statistics via a stats.Store. Stats recording errors are silently
// ignored so they never affect the underlying storage operation.
type Backend struct {
	inner storage.Backend
	store *stats.Store
}

// New returns a stats-tracking wrapper around the given backend.
func New(inner storage.Backend, store *stats.Store) *Backend {
	return &Backend{inner: inner, store: store}
}

func repoPath(ctx context.Context) string {
	rp := middleware.GetRepoPrefix(ctx)
	if rp == "" {
		return "/"
	}
	return rp
}

func (b *Backend) CreateRepo(ctx context.Context) error {
	return b.inner.CreateRepo(ctx)
}

func (b *Backend) DeleteRepo(ctx context.Context) error {
	return b.inner.DeleteRepo(ctx)
}

func (b *Backend) StatConfig(ctx context.Context) (int64, error) {
	return b.inner.StatConfig(ctx)
}

func (b *Backend) GetConfig(ctx context.Context) (io.ReadCloser, error) {
	rc, err := b.inner.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	repo := repoPath(ctx)
	return &statsReadCloser{
		rc:      rc,
		onClose: func(n int64) { b.store.RecordRead(repo, n) },
	}, nil
}

func (b *Backend) SaveConfig(ctx context.Context, data io.Reader) error {
	cr := &countingReader{r: data}
	err := b.inner.SaveConfig(ctx, cr)
	if err == nil {
		b.store.RecordWrite(repoPath(ctx), cr.count)
	}
	return err
}

func (b *Backend) StatBlob(ctx context.Context, t storage.BlobType, name string) (int64, error) {
	return b.inner.StatBlob(ctx, t, name)
}

func (b *Backend) GetBlob(ctx context.Context, t storage.BlobType, name string, offset, length int64) (io.ReadCloser, error) {
	rc, err := b.inner.GetBlob(ctx, t, name, offset, length)
	if err != nil {
		return nil, err
	}
	repo := repoPath(ctx)
	return &statsReadCloser{
		rc:      rc,
		onClose: func(n int64) { b.store.RecordRead(repo, n) },
	}, nil
}

func (b *Backend) SaveBlob(ctx context.Context, t storage.BlobType, name string, data io.Reader) error {
	cr := &countingReader{r: data}
	err := b.inner.SaveBlob(ctx, t, name, cr)
	if err == nil {
		b.store.RecordWrite(repoPath(ctx), cr.count)
	}
	return err
}

func (b *Backend) DeleteBlob(ctx context.Context, t storage.BlobType, name string) error {
	// Get size before deleting for accurate byte tracking.
	size, _ := b.inner.StatBlob(ctx, t, name)
	err := b.inner.DeleteBlob(ctx, t, name)
	if err == nil {
		b.store.RecordDelete(repoPath(ctx), size)
	}
	return err
}

func (b *Backend) ListBlobs(ctx context.Context, t storage.BlobType) ([]storage.Blob, error) {
	return b.inner.ListBlobs(ctx, t)
}

// countingReader wraps an io.Reader and counts bytes read through it.
type countingReader struct {
	r     io.Reader
	count int64
}

func (cr *countingReader) Read(p []byte) (int, error) {
	n, err := cr.r.Read(p)
	cr.count += int64(n)
	return n, err
}

// statsReadCloser wraps an io.ReadCloser and calls onClose with the
// total bytes read when Close is called.
type statsReadCloser struct {
	rc      io.ReadCloser
	count   int64
	onClose func(int64)
}

func (s *statsReadCloser) Read(p []byte) (int, error) {
	n, err := s.rc.Read(p)
	s.count += int64(n)
	return n, err
}

func (s *statsReadCloser) Close() error {
	if s.onClose != nil {
		s.onClose(s.count)
	}
	return s.rc.Close()
}
