package memory

import (
	"bytes"
	"context"
	"io"
	"sync"

	"github.com/chrismcg/ts-restic-server/internal/middleware"
	"github.com/chrismcg/ts-restic-server/internal/storage"
)

type repo struct {
	config []byte
	blobs  map[storage.BlobType]map[string][]byte
}

type Backend struct {
	mu       sync.RWMutex
	maxBytes int64
	usedBytes int64
	repos    map[string]*repo // keyed by repo prefix
}

func New(maxBytes int64) *Backend {
	return &Backend{
		maxBytes: maxBytes,
		repos:    make(map[string]*repo),
	}
}

func repoKey(ctx context.Context) string {
	return middleware.GetRepoPrefix(ctx)
}

func (b *Backend) CreateRepo(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := repoKey(ctx)
	if _, ok := b.repos[key]; ok {
		return storage.ErrRepoExists
	}

	r := &repo{
		blobs: make(map[storage.BlobType]map[string][]byte),
	}
	for t := range storage.ValidBlobTypes {
		r.blobs[t] = make(map[string][]byte)
	}
	b.repos[key] = r
	return nil
}

func (b *Backend) DeleteRepo(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := repoKey(ctx)
	r, ok := b.repos[key]
	if !ok {
		return storage.ErrRepoNotFound
	}

	// Reclaim used bytes
	if r.config != nil {
		b.usedBytes -= int64(len(r.config))
	}
	for _, bucket := range r.blobs {
		for _, data := range bucket {
			b.usedBytes -= int64(len(data))
		}
	}

	delete(b.repos, key)
	return nil
}

func (b *Backend) getRepo(ctx context.Context) (*repo, bool) {
	r, ok := b.repos[repoKey(ctx)]
	return r, ok
}

func (b *Backend) StatConfig(ctx context.Context) (int64, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	r, ok := b.getRepo(ctx)
	if !ok || r.config == nil {
		return 0, storage.ErrNotFound
	}
	return int64(len(r.config)), nil
}

func (b *Backend) GetConfig(ctx context.Context) (io.ReadCloser, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	r, ok := b.getRepo(ctx)
	if !ok || r.config == nil {
		return nil, storage.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(r.config)), nil
}

func (b *Backend) SaveConfig(ctx context.Context, data io.Reader) error {
	buf, err := io.ReadAll(data)
	if err != nil {
		return err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	r, ok := b.getRepo(ctx)
	if !ok {
		return storage.ErrRepoNotFound
	}

	oldSize := int64(len(r.config))
	newSize := int64(len(buf))
	if b.usedBytes-oldSize+newSize > b.maxBytes {
		return storage.ErrQuotaExceeded
	}

	b.usedBytes = b.usedBytes - oldSize + newSize
	r.config = buf
	return nil
}

func (b *Backend) StatBlob(ctx context.Context, t storage.BlobType, name string) (int64, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	r, ok := b.getRepo(ctx)
	if !ok {
		return 0, storage.ErrNotFound
	}
	bucket, ok := r.blobs[t]
	if !ok {
		return 0, storage.ErrNotFound
	}
	data, ok := bucket[name]
	if !ok {
		return 0, storage.ErrNotFound
	}
	return int64(len(data)), nil
}

func (b *Backend) GetBlob(ctx context.Context, t storage.BlobType, name string, offset, length int64) (io.ReadCloser, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	r, ok := b.getRepo(ctx)
	if !ok {
		return nil, storage.ErrNotFound
	}
	bucket, ok := r.blobs[t]
	if !ok {
		return nil, storage.ErrNotFound
	}
	data, ok := bucket[name]
	if !ok {
		return nil, storage.ErrNotFound
	}

	if offset >= int64(len(data)) {
		return io.NopCloser(bytes.NewReader(nil)), nil
	}

	end := int64(len(data))
	if length > 0 && offset+length < end {
		end = offset + length
	}
	return io.NopCloser(bytes.NewReader(data[offset:end])), nil
}

func (b *Backend) SaveBlob(ctx context.Context, t storage.BlobType, name string, data io.Reader) error {
	buf, err := io.ReadAll(data)
	if err != nil {
		return err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	r, ok := b.getRepo(ctx)
	if !ok {
		return storage.ErrRepoNotFound
	}
	bucket, ok := r.blobs[t]
	if !ok {
		return storage.ErrNotFound
	}

	newSize := int64(len(buf))
	oldSize := int64(0)
	if existing, ok := bucket[name]; ok {
		oldSize = int64(len(existing))
	}

	if b.usedBytes-oldSize+newSize > b.maxBytes {
		return storage.ErrQuotaExceeded
	}

	b.usedBytes = b.usedBytes - oldSize + newSize
	bucket[name] = buf
	return nil
}

func (b *Backend) DeleteBlob(ctx context.Context, t storage.BlobType, name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	r, ok := b.getRepo(ctx)
	if !ok {
		return storage.ErrNotFound
	}
	bucket, ok := r.blobs[t]
	if !ok {
		return storage.ErrNotFound
	}
	data, ok := bucket[name]
	if !ok {
		return storage.ErrNotFound
	}

	b.usedBytes -= int64(len(data))
	delete(bucket, name)
	return nil
}

func (b *Backend) ListBlobs(ctx context.Context, t storage.BlobType) ([]storage.Blob, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	r, ok := b.getRepo(ctx)
	if !ok {
		return nil, nil
	}
	bucket, ok := r.blobs[t]
	if !ok {
		return nil, nil
	}

	blobs := make([]storage.Blob, 0, len(bucket))
	for name, data := range bucket {
		blobs = append(blobs, storage.Blob{Name: name, Size: int64(len(data))})
	}
	return blobs, nil
}
