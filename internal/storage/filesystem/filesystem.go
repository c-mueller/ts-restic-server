package filesystem

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/chrismcg/ts-restic-server/internal/storage"
)

type Backend struct {
	basePath string
}

func New(basePath string) (*Backend, error) {
	return &Backend{basePath: basePath}, nil
}

func (b *Backend) CreateRepo(_ context.Context) error {
	dirs := []string{
		b.basePath,
		filepath.Join(b.basePath, "keys"),
		filepath.Join(b.basePath, "locks"),
		filepath.Join(b.basePath, "snapshots"),
		filepath.Join(b.basePath, "index"),
	}

	// Create data/00 - data/ff subdirectories
	for i := 0; i < 256; i++ {
		dirs = append(dirs, filepath.Join(b.basePath, "data", fmt.Sprintf("%02x", i)))
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	return nil
}

func (b *Backend) DeleteRepo(_ context.Context) error {
	if _, err := os.Stat(b.basePath); os.IsNotExist(err) {
		return storage.ErrRepoNotFound
	}
	return os.RemoveAll(b.basePath)
}

func (b *Backend) StatConfig(_ context.Context) (int64, error) {
	return statFile(b.configPath())
}

func (b *Backend) GetConfig(_ context.Context) (io.ReadCloser, error) {
	f, err := os.Open(b.configPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	return f, nil
}

func (b *Backend) SaveConfig(_ context.Context, data io.Reader) error {
	return atomicWrite(b.configPath(), data)
}

func (b *Backend) StatBlob(_ context.Context, t storage.BlobType, name string) (int64, error) {
	return statFile(b.blobPath(t, name))
}

func (b *Backend) GetBlob(_ context.Context, t storage.BlobType, name string, offset, length int64) (io.ReadCloser, error) {
	f, err := os.Open(b.blobPath(t, name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}

	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			f.Close()
			return nil, err
		}
	}

	if length > 0 {
		return &limitedReadCloser{Reader: io.LimitReader(f, length), Closer: f}, nil
	}

	return f, nil
}

func (b *Backend) SaveBlob(_ context.Context, t storage.BlobType, name string, data io.Reader) error {
	path := b.blobPath(t, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return atomicWrite(path, data)
}

func (b *Backend) DeleteBlob(_ context.Context, t storage.BlobType, name string) error {
	err := os.Remove(b.blobPath(t, name))
	if err != nil {
		if os.IsNotExist(err) {
			return storage.ErrNotFound
		}
		return err
	}
	return nil
}

func (b *Backend) ListBlobs(_ context.Context, t storage.BlobType) ([]storage.Blob, error) {
	dir := b.typePath(t)
	var blobs []storage.Blob

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		blobs = append(blobs, storage.Blob{
			Name: info.Name(),
			Size: info.Size(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if blobs == nil {
		blobs = []storage.Blob{}
	}
	return blobs, nil
}

func (b *Backend) configPath() string {
	return filepath.Join(b.basePath, "config")
}

func (b *Backend) typePath(t storage.BlobType) string {
	return filepath.Join(b.basePath, string(t))
}

func (b *Backend) blobPath(t storage.BlobType, name string) string {
	if t == storage.BlobData && len(name) >= 2 {
		return filepath.Join(b.basePath, "data", name[:2], name)
	}
	return filepath.Join(b.basePath, string(t), name)
}

func statFile(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, storage.ErrNotFound
		}
		return 0, err
	}
	return info.Size(), nil
}

func atomicWrite(path string, data io.Reader) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	defer func() {
		if tmp != nil {
			tmp.Close()
			os.Remove(tmpPath)
		}
	}()

	if _, err := io.Copy(tmp, data); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("fsync temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	tmp = nil // prevent deferred cleanup

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp to final: %w", err)
	}

	return nil
}

type limitedReadCloser struct {
	io.Reader
	io.Closer
}
