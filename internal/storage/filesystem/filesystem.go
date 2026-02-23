package filesystem

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/c-mueller/ts-restic-server/internal/middleware"
	"github.com/c-mueller/ts-restic-server/internal/storage"
)

type Backend struct {
	basePath     string
	dataSharding bool
}

func New(basePath string, dataSharding bool) (*Backend, error) {
	if err := os.MkdirAll(basePath, 0o700); err != nil {
		return nil, fmt.Errorf("create storage directory %s: %w", basePath, err)
	}
	return &Backend{basePath: basePath, dataSharding: dataSharding}, nil
}

func (b *Backend) CreateRepo(ctx context.Context) error {
	rp := b.repoPath(ctx)
	dirs := []string{
		rp,
		filepath.Join(rp, "keys"),
		filepath.Join(rp, "locks"),
		filepath.Join(rp, "snapshots"),
		filepath.Join(rp, "index"),
	}

	if b.dataSharding {
		// Create data/00 - data/ff subdirectories
		for i := 0; i < 256; i++ {
			dirs = append(dirs, filepath.Join(rp, "data", fmt.Sprintf("%02x", i)))
		}
	} else {
		dirs = append(dirs, filepath.Join(rp, "data"))
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	return nil
}

func (b *Backend) DeleteRepo(ctx context.Context) error {
	rp := b.repoPath(ctx)
	if _, err := os.Stat(rp); os.IsNotExist(err) {
		return storage.ErrRepoNotFound
	}
	return os.RemoveAll(rp)
}

func (b *Backend) StatConfig(ctx context.Context) (int64, error) {
	return statFile(b.configPath(ctx))
}

func (b *Backend) GetConfig(ctx context.Context) (io.ReadCloser, error) {
	f, err := os.Open(b.configPath(ctx))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	return f, nil
}

func (b *Backend) SaveConfig(ctx context.Context, data io.Reader) error {
	dir := filepath.Dir(b.configPath(ctx))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return atomicWrite(b.configPath(ctx), data)
}

func (b *Backend) StatBlob(ctx context.Context, t storage.BlobType, name string) (int64, error) {
	return statFile(b.blobPath(ctx, t, name))
}

func (b *Backend) GetBlob(ctx context.Context, t storage.BlobType, name string, offset, length int64) (io.ReadCloser, error) {
	f, err := os.Open(b.blobPath(ctx, t, name))
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

func (b *Backend) SaveBlob(ctx context.Context, t storage.BlobType, name string, data io.Reader) error {
	path := b.blobPath(ctx, t, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return atomicWrite(path, data)
}

func (b *Backend) DeleteBlob(ctx context.Context, t storage.BlobType, name string) error {
	err := os.Remove(b.blobPath(ctx, t, name))
	if err != nil {
		if os.IsNotExist(err) {
			return storage.ErrNotFound
		}
		return err
	}
	return nil
}

func (b *Backend) ListBlobs(ctx context.Context, t storage.BlobType) ([]storage.Blob, error) {
	dir := b.typePath(ctx, t)
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

func (b *Backend) repoPath(ctx context.Context) string {
	if rp := middleware.GetRepoPrefix(ctx); rp != "" {
		return filepath.Join(b.basePath, rp)
	}
	return b.basePath
}

func (b *Backend) configPath(ctx context.Context) string {
	return filepath.Join(b.repoPath(ctx), "config")
}

func (b *Backend) typePath(ctx context.Context, t storage.BlobType) string {
	return filepath.Join(b.repoPath(ctx), string(t))
}

func (b *Backend) blobPath(ctx context.Context, t storage.BlobType, name string) string {
	if t == storage.BlobData && b.dataSharding && len(name) >= 2 {
		return filepath.Join(b.repoPath(ctx), "data", name[:2], name)
	}
	return filepath.Join(b.repoPath(ctx), string(t), name)
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
