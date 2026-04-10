package filesystem

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/c-mueller/ts-restic-server/internal/middleware"
	"github.com/c-mueller/ts-restic-server/internal/storage"
)

// ErrPathEscape is returned when a resolved path escapes the storage base directory,
// which can happen if symbolic links point outside the storage tree.
var ErrPathEscape = errors.New("path escapes storage directory")

type Backend struct {
	basePath     string
	resolvedBase string
}

func New(basePath string) (*Backend, error) {
	if err := os.MkdirAll(basePath, 0o700); err != nil {
		return nil, fmt.Errorf("create storage directory %s: %w", basePath, err)
	}
	resolved, err := filepath.EvalSymlinks(basePath)
	if err != nil {
		return nil, fmt.Errorf("resolve storage directory %s: %w", basePath, err)
	}
	return &Backend{basePath: basePath, resolvedBase: resolved}, nil
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

	// Create data/00 - data/ff subdirectories (restic-server compatible layout)
	for i := 0; i < 256; i++ {
		dirs = append(dirs, filepath.Join(rp, "data", fmt.Sprintf("%02x", i)))
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
	if err := b.validatePath(rp); err != nil {
		return err
	}
	if _, err := os.Stat(rp); os.IsNotExist(err) {
		return storage.ErrRepoNotFound
	}
	return os.RemoveAll(rp)
}

func (b *Backend) StatConfig(ctx context.Context) (int64, error) {
	p := b.configPath(ctx)
	if err := b.validatePath(p); err != nil {
		return 0, err
	}
	return statFile(p)
}

func (b *Backend) GetConfig(ctx context.Context) (io.ReadCloser, error) {
	p := b.configPath(ctx)
	if err := b.validatePath(p); err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	return f, nil
}

func (b *Backend) SaveConfig(ctx context.Context, data io.Reader) error {
	p := b.configPath(ctx)
	if err := b.validatePath(p); err != nil {
		return err
	}
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return atomicWrite(p, data)
}

func (b *Backend) StatBlob(ctx context.Context, t storage.BlobType, name string) (int64, error) {
	p := b.blobPath(ctx, t, name)
	if err := b.validatePath(p); err != nil {
		return 0, err
	}
	size, err := statFile(p)
	if err == storage.ErrNotFound && t == storage.BlobData && len(name) >= 2 {
		// Fallback: check unsharded path for pre-sharding data.
		up := b.blobPathUnsharded(ctx, name)
		if verr := b.validatePath(up); verr != nil {
			return 0, err
		}
		return statFile(up)
	}
	return size, err
}

func (b *Backend) GetBlob(ctx context.Context, t storage.BlobType, name string, offset, length int64) (io.ReadCloser, error) {
	p := b.blobPath(ctx, t, name)
	if err := b.validatePath(p); err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) && t == storage.BlobData && len(name) >= 2 {
			// Fallback: try unsharded path for pre-sharding data.
			up := b.blobPathUnsharded(ctx, name)
			if verr := b.validatePath(up); verr == nil {
				f, err = os.Open(up)
			}
		}
		if err != nil {
			if os.IsNotExist(err) {
				return nil, storage.ErrNotFound
			}
			return nil, err
		}
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
	p := b.blobPath(ctx, t, name)
	if err := b.validatePath(p); err != nil {
		return err
	}

	// Content-addressed storage: if the blob already exists, skip the write.
	// Both concurrent writers produce identical data for the same hash, so
	// the existing file is already correct. Drain the reader to avoid
	// connection issues on the caller side.
	if _, err := os.Lstat(p); err == nil {
		io.Copy(io.Discard, data)
		return nil
	}
	// Also check unsharded path for pre-sharding data.
	if t == storage.BlobData && len(name) >= 2 {
		up := b.blobPathUnsharded(ctx, name)
		if _, err := os.Lstat(up); err == nil {
			io.Copy(io.Discard, data)
			return nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	return atomicWrite(p, data)
}

func (b *Backend) DeleteBlob(ctx context.Context, t storage.BlobType, name string) error {
	p := b.blobPath(ctx, t, name)
	if err := b.validatePath(p); err != nil {
		return err
	}
	err := os.Remove(p)
	if err != nil {
		if os.IsNotExist(err) && t == storage.BlobData && len(name) >= 2 {
			// Fallback: try unsharded path for pre-sharding data.
			up := b.blobPathUnsharded(ctx, name)
			if verr := b.validatePath(up); verr == nil {
				err = os.Remove(up)
			}
		}
		if err != nil {
			if os.IsNotExist(err) {
				return storage.ErrNotFound
			}
			return err
		}
	}
	return nil
}

func (b *Backend) ListBlobs(ctx context.Context, t storage.BlobType) ([]storage.Blob, error) {
	dir := b.typePath(ctx, t)
	if err := b.validatePath(dir); err != nil {
		return nil, err
	}
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
		// Skip symlinks — only regular files are valid blobs.
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		// info.Name() returns the bare filename without any directory prefix.
		// When data sharding is enabled, blobs live in subdirectories like
		// data/ab/abcdef..., but filepath.Walk traverses into those shard
		// directories automatically. The restic REST API expects only the
		// bare hash name, and blobPath() reconstructs the shard prefix from
		// the first two characters when needed.
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
	if t == storage.BlobData && len(name) >= 2 {
		return filepath.Join(b.repoPath(ctx), "data", name[:2], name)
	}
	return filepath.Join(b.repoPath(ctx), string(t), name)
}

// blobPathUnsharded returns the flat (non-sharded) path for a data blob.
// Used as fallback when reading blobs that were stored before sharding was enabled.
func (b *Backend) blobPathUnsharded(ctx context.Context, name string) string {
	return filepath.Join(b.repoPath(ctx), "data", name)
}

// validatePath resolves symlinks in path and verifies it stays within the
// storage base directory. For paths that don't exist yet (new files), the
// parent directory is validated instead.
func (b *Backend) validatePath(path string) error {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet — validate parent directory.
			dir := filepath.Dir(path)
			resolved, err = filepath.EvalSymlinks(dir)
			if err != nil {
				return fmt.Errorf("resolve parent directory: %w", err)
			}
			if !isSubPath(resolved, b.resolvedBase) {
				return ErrPathEscape
			}
			return nil
		}
		return err
	}
	if !isSubPath(resolved, b.resolvedBase) {
		return ErrPathEscape
	}
	return nil
}

// isSubPath reports whether path is equal to or a subdirectory of base.
func isSubPath(path, base string) bool {
	return path == base || strings.HasPrefix(path, base+string(filepath.Separator))
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
