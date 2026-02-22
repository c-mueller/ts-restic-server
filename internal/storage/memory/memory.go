package memory

import (
	"bytes"
	"context"
	"io"
	"sync"

	"github.com/chrismcg/ts-restic-server/internal/storage"
)

type Backend struct {
	mu            sync.RWMutex
	maxBytes      int64
	usedBytes     int64
	repoExists    bool
	config        []byte
	blobs         map[storage.BlobType]map[string][]byte
}

func New(maxBytes int64) *Backend {
	return &Backend{
		maxBytes: maxBytes,
		blobs:    make(map[storage.BlobType]map[string][]byte),
	}
}

func (b *Backend) CreateRepo(_ context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.repoExists {
		return storage.ErrRepoExists
	}

	b.repoExists = true
	b.blobs = make(map[storage.BlobType]map[string][]byte)
	for t := range storage.ValidBlobTypes {
		b.blobs[t] = make(map[string][]byte)
	}
	b.config = nil
	b.usedBytes = 0
	return nil
}

func (b *Backend) DeleteRepo(_ context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.repoExists {
		return storage.ErrRepoNotFound
	}

	b.repoExists = false
	b.blobs = make(map[storage.BlobType]map[string][]byte)
	b.config = nil
	b.usedBytes = 0
	return nil
}

func (b *Backend) StatConfig(_ context.Context) (int64, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.config == nil {
		return 0, storage.ErrNotFound
	}
	return int64(len(b.config)), nil
}

func (b *Backend) GetConfig(_ context.Context) (io.ReadCloser, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.config == nil {
		return nil, storage.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(b.config)), nil
}

func (b *Backend) SaveConfig(_ context.Context, data io.Reader) error {
	buf, err := io.ReadAll(data)
	if err != nil {
		return err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	oldSize := int64(len(b.config))
	newSize := int64(len(buf))
	if b.usedBytes-oldSize+newSize > b.maxBytes {
		return storage.ErrQuotaExceeded
	}

	b.usedBytes = b.usedBytes - oldSize + newSize
	b.config = buf
	return nil
}

func (b *Backend) StatBlob(_ context.Context, t storage.BlobType, name string) (int64, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	bucket, ok := b.blobs[t]
	if !ok {
		return 0, storage.ErrNotFound
	}
	data, ok := bucket[name]
	if !ok {
		return 0, storage.ErrNotFound
	}
	return int64(len(data)), nil
}

func (b *Backend) GetBlob(_ context.Context, t storage.BlobType, name string, offset, length int64) (io.ReadCloser, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	bucket, ok := b.blobs[t]
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

func (b *Backend) SaveBlob(_ context.Context, t storage.BlobType, name string, data io.Reader) error {
	buf, err := io.ReadAll(data)
	if err != nil {
		return err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	bucket, ok := b.blobs[t]
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

func (b *Backend) DeleteBlob(_ context.Context, t storage.BlobType, name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	bucket, ok := b.blobs[t]
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

func (b *Backend) ListBlobs(_ context.Context, t storage.BlobType) ([]storage.Blob, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	bucket, ok := b.blobs[t]
	if !ok {
		return nil, nil
	}

	blobs := make([]storage.Blob, 0, len(bucket))
	for name, data := range bucket {
		blobs = append(blobs, storage.Blob{Name: name, Size: int64(len(data))})
	}
	return blobs, nil
}
