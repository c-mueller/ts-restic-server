package webdav

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/c-mueller/ts-restic-server/internal/middleware"
	"github.com/c-mueller/ts-restic-server/internal/storage"
	"github.com/studio-b12/gowebdav"
)

type Backend struct {
	client *gowebdav.Client
	prefix string
}

func New(endpoint, username, password, prefix string) *Backend {
	client := gowebdav.NewClient(endpoint, username, password)
	return &Backend{
		client: client,
		prefix: strings.TrimSuffix(prefix, "/"),
	}
}

func (b *Backend) CreateRepo(ctx context.Context) error {
	types := []string{"keys", "locks", "snapshots", "index", "data"}
	for _, t := range types {
		dir := b.pathJoin(ctx, t)
		if err := b.client.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	// Create data/00 - data/ff subdirectories (restic-server compatible layout)
	for i := 0; i < 256; i++ {
		dir := b.pathJoin(ctx, "data", fmt.Sprintf("%02x", i))
		if err := b.client.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}

func (b *Backend) DeleteRepo(ctx context.Context) error {
	dir := b.repoDir(ctx)
	if dir == "" || dir == "/" {
		// Safety: don't delete root
		return errors.New("cannot delete root directory")
	}
	return b.client.RemoveAll(dir)
}

func (b *Backend) StatConfig(ctx context.Context) (int64, error) {
	p := b.pathJoin(ctx, "config")
	fi, err := b.client.Stat(p)
	if err != nil {
		if isNotFound(err) {
			return 0, storage.ErrNotFound
		}
		return 0, err
	}
	return fi.Size(), nil
}

func (b *Backend) GetConfig(ctx context.Context) (io.ReadCloser, error) {
	p := b.pathJoin(ctx, "config")
	rc, err := b.client.ReadStream(p)
	if err != nil {
		if isNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	return rc, nil
}

func (b *Backend) SaveConfig(ctx context.Context, data io.Reader) error {
	p := b.pathJoin(ctx, "config")
	return b.client.WriteStream(p, data, 0644)
}

func (b *Backend) StatBlob(ctx context.Context, t storage.BlobType, name string) (int64, error) {
	p := b.blobPath(ctx, t, name)
	fi, err := b.client.Stat(p)
	if err != nil {
		if isNotFound(err) && t == storage.BlobData && len(name) >= 2 {
			// Fallback: try unsharded path for pre-sharding data.
			up := b.blobPathUnsharded(ctx, name)
			fi, err = b.client.Stat(up)
			if err != nil {
				if isNotFound(err) {
					return 0, storage.ErrNotFound
				}
				return 0, err
			}
			return fi.Size(), nil
		}
		if isNotFound(err) {
			return 0, storage.ErrNotFound
		}
		return 0, err
	}
	return fi.Size(), nil
}

func (b *Backend) GetBlob(ctx context.Context, t storage.BlobType, name string, offset, length int64) (io.ReadCloser, error) {
	p := b.blobPath(ctx, t, name)

	// tryRead attempts to read from the given path, with optional range support.
	tryRead := func(fp string) (io.ReadCloser, error) {
		if offset > 0 || length > 0 {
			return b.client.ReadStreamRange(fp, offset, length)
		}
		return b.client.ReadStream(fp)
	}

	rc, err := tryRead(p)
	if err != nil {
		if isNotFound(err) && t == storage.BlobData && len(name) >= 2 {
			// Fallback: try unsharded path for pre-sharding data.
			up := b.blobPathUnsharded(ctx, name)
			rc, err = tryRead(up)
		}
		if err != nil {
			if isNotFound(err) {
				return nil, storage.ErrNotFound
			}
			return nil, err
		}
	}
	return rc, nil
}

func (b *Backend) SaveBlob(ctx context.Context, t storage.BlobType, name string, data io.Reader) error {
	p := b.blobPath(ctx, t, name)
	dir := path.Dir(p)
	if err := b.client.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return b.client.WriteStream(p, data, 0644)
}

func (b *Backend) DeleteBlob(ctx context.Context, t storage.BlobType, name string) error {
	p := b.blobPath(ctx, t, name)
	err := b.client.Remove(p)
	if err != nil {
		if isNotFound(err) && t == storage.BlobData && len(name) >= 2 {
			// Fallback: try unsharded path for pre-sharding data.
			up := b.blobPathUnsharded(ctx, name)
			if err := b.client.Remove(up); err != nil {
				if isNotFound(err) {
					return storage.ErrNotFound
				}
				return err
			}
			return nil
		}
		if isNotFound(err) {
			return storage.ErrNotFound
		}
		return err
	}
	return nil
}

func (b *Backend) ListBlobs(ctx context.Context, t storage.BlobType) ([]storage.Blob, error) {
	dir := b.pathJoin(ctx, string(t))

	if t == storage.BlobData {
		return b.listShardedBlobs(dir)
	}

	return b.listDir(dir)
}

// listDir lists blobs in a single directory.
func (b *Backend) listDir(dir string) ([]storage.Blob, error) {
	files, err := b.client.ReadDir(dir)
	if err != nil {
		if isNotFound(err) {
			return []storage.Blob{}, nil
		}
		return nil, err
	}

	blobs := make([]storage.Blob, 0, len(files))
	for _, fi := range files {
		if fi.IsDir() {
			continue
		}
		blobs = append(blobs, storage.Blob{
			Name: fi.Name(),
			Size: fi.Size(),
		})
	}
	return blobs, nil
}

// listShardedBlobs iterates the 256 shard subdirectories and collects all blobs.
// It also collects any blobs stored directly in dataDir (pre-sharding fallback).
func (b *Backend) listShardedBlobs(dataDir string) ([]storage.Blob, error) {
	// Collect blobs from flat data/ directory (pre-sharding fallback).
	blobs, err := b.listDir(dataDir)
	if err != nil {
		return nil, err
	}
	for i := 0; i < 256; i++ {
		subdir := path.Join(dataDir, fmt.Sprintf("%02x", i))
		sub, err := b.listDir(subdir)
		if err != nil {
			return nil, err
		}
		blobs = append(blobs, sub...)
	}
	if blobs == nil {
		blobs = []storage.Blob{}
	}
	return blobs, nil
}

// repoPrefix returns the combined config prefix + request repo prefix.
func (b *Backend) repoPrefix(ctx context.Context) string {
	parts := []string{}
	if b.prefix != "" {
		parts = append(parts, b.prefix)
	}
	if rp := middleware.GetRepoPrefix(ctx); rp != "" {
		parts = append(parts, rp)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "/")
}

// repoDir returns the repo directory path with a trailing slash.
func (b *Backend) repoDir(ctx context.Context) string {
	p := b.repoPrefix(ctx)
	if p == "" {
		return "/"
	}
	return "/" + p + "/"
}

// pathJoin constructs a full WebDAV path from the repo prefix and sub-parts.
func (b *Backend) pathJoin(ctx context.Context, parts ...string) string {
	sub := path.Join(parts...)
	if p := b.repoPrefix(ctx); p != "" {
		return "/" + p + "/" + sub
	}
	return "/" + sub
}

// blobPath returns the full path for a blob.
func (b *Backend) blobPath(ctx context.Context, t storage.BlobType, name string) string {
	if t == storage.BlobData && len(name) >= 2 {
		return b.pathJoin(ctx, "data", name[:2], name)
	}
	return b.pathJoin(ctx, string(t), name)
}

// blobPathUnsharded returns the flat (non-sharded) path for a data blob.
// Used as fallback when reading blobs that were stored before sharding was enabled.
func (b *Backend) blobPathUnsharded(ctx context.Context, name string) string {
	return b.pathJoin(ctx, "data", name)
}

// isNotFound checks if the error indicates a 404 / not-found condition.
func isNotFound(err error) bool {
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	var pe *os.PathError
	if errors.As(err, &pe) && errors.Is(pe.Err, os.ErrNotExist) {
		return true
	}
	// gowebdav may return errors containing "404" or "Not Found"
	s := err.Error()
	return strings.Contains(s, "404") || strings.Contains(s, "Not Found")
}
