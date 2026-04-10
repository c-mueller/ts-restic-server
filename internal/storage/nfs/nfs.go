package nfs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/c-mueller/ts-restic-server/internal/middleware"
	"github.com/c-mueller/ts-restic-server/internal/storage"
	gonfs "github.com/willscott/go-nfs-client/nfs"
	"github.com/willscott/go-nfs-client/nfs/rpc"
)

// Backend implements storage.Backend using an NFSv3 export.
// It uses a pure-Go NFS client and does not require OS-level mounting.
type Backend struct {
	server   string
	export   string
	basePath string
	uid      uint32
	gid      uint32

	mu     sync.Mutex
	mount  *gonfs.Mount
	target *gonfs.Target
}

// New creates and connects a new NFS backend.
func New(server, export, basePath string, uid, gid uint32) (*Backend, error) {
	b := &Backend{
		server:   server,
		export:   export,
		basePath: basePath,
		uid:      uid,
		gid:      gid,
	}
	if err := b.connect(); err != nil {
		return nil, fmt.Errorf("nfs connect: %w", err)
	}
	return b, nil
}

func (b *Backend) connect() error {
	mount, err := gonfs.DialMount(b.server, time.Second*10)
	if err != nil {
		return fmt.Errorf("dial mount %s: %w", b.server, err)
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "ts-restic-server"
	}
	auth := rpc.NewAuthUnix(hostname, b.uid, b.gid)

	target, err := mount.Mount(b.export, auth.Auth())
	if err != nil {
		mount.Close()
		return fmt.Errorf("mount export %q: %w", b.export, err)
	}

	b.mount = mount
	b.target = target
	return nil
}

// ensureConnected reconnects if the NFS session has been lost.
func (b *Backend) ensureConnected() error {
	if b.target != nil {
		// Quick health check: stat the base path (or root).
		checkPath := b.basePath
		if checkPath == "" {
			checkPath = "."
		}
		if _, err := b.target.Getattr(checkPath); err == nil {
			return nil
		}
	}
	b.disconnect()
	return b.connect()
}

func (b *Backend) disconnect() {
	if b.target != nil {
		b.target.Close()
		b.target = nil
	}
	if b.mount != nil {
		b.mount.Unmount()
		b.mount.Close()
		b.mount = nil
	}
}

// withTarget acquires the mutex, ensures the connection is live, and calls fn
// with the connected target. All backend methods use this to serialize access
// and handle reconnection transparently.
func (b *Backend) withTarget(fn func(t *gonfs.Target) error) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.ensureConnected(); err != nil {
		return err
	}
	return fn(b.target)
}

// Close releases the NFS connection.
func (b *Backend) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.disconnect()
}

func (b *Backend) repoPath(ctx context.Context) string {
	rp := middleware.GetRepoPrefix(ctx)
	if b.basePath != "" && rp != "" {
		return path.Join(b.basePath, rp)
	}
	if b.basePath != "" {
		return b.basePath
	}
	if rp != "" {
		return rp
	}
	return "."
}

func (b *Backend) configPath(ctx context.Context) string {
	return path.Join(b.repoPath(ctx), "config")
}

func (b *Backend) typePath(ctx context.Context, t storage.BlobType) string {
	return path.Join(b.repoPath(ctx), string(t))
}

func (b *Backend) blobPath(ctx context.Context, t storage.BlobType, name string) string {
	if t == storage.BlobData && len(name) >= 2 {
		return path.Join(b.repoPath(ctx), "data", name[:2], name)
	}
	return path.Join(b.repoPath(ctx), string(t), name)
}

// blobPathUnsharded returns the flat (non-sharded) path for a data blob.
// Used as fallback when reading blobs that were stored before sharding was enabled.
func (b *Backend) blobPathUnsharded(ctx context.Context, name string) string {
	return path.Join(b.repoPath(ctx), "data", name)
}

func (b *Backend) CreateRepo(ctx context.Context) error {
	rp := b.repoPath(ctx)
	dirs := []string{
		rp,
		path.Join(rp, "keys"),
		path.Join(rp, "locks"),
		path.Join(rp, "snapshots"),
		path.Join(rp, "index"),
	}
	// Create data/00 - data/ff subdirectories (restic-server compatible layout)
	for i := 0; i < 256; i++ {
		dirs = append(dirs, path.Join(rp, "data", fmt.Sprintf("%02x", i)))
	}

	return b.withTarget(func(t *gonfs.Target) error {
		for _, dir := range dirs {
			if err := mkdirAll(t, dir); err != nil {
				return fmt.Errorf("create directory %s: %w", dir, err)
			}
		}
		return nil
	})
}

func (b *Backend) DeleteRepo(ctx context.Context) error {
	rp := b.repoPath(ctx)
	return b.withTarget(func(t *gonfs.Target) error {
		if _, err := t.Getattr(rp); err != nil {
			if isNotFound(err) {
				return storage.ErrRepoNotFound
			}
			return err
		}
		return t.RemoveAll(rp)
	})
}

func (b *Backend) StatConfig(ctx context.Context) (int64, error) {
	p := b.configPath(ctx)
	var size int64
	err := b.withTarget(func(t *gonfs.Target) error {
		attr, err := t.Getattr(p)
		if err != nil {
			if isNotFound(err) {
				return storage.ErrNotFound
			}
			return err
		}
		size = attr.Size()
		return nil
	})
	return size, err
}

func (b *Backend) GetConfig(ctx context.Context) (io.ReadCloser, error) {
	p := b.configPath(ctx)
	var buf []byte
	err := b.withTarget(func(t *gonfs.Target) error {
		f, err := t.Open(p)
		if err != nil {
			if isNotFound(err) {
				return storage.ErrNotFound
			}
			return err
		}
		defer f.Close()
		buf, err = io.ReadAll(f)
		return err
	})
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(buf)), nil
}

func (b *Backend) SaveConfig(ctx context.Context, data io.Reader) error {
	p := b.configPath(ctx)
	content, err := io.ReadAll(data)
	if err != nil {
		return fmt.Errorf("read config data: %w", err)
	}
	return b.withTarget(func(t *gonfs.Target) error {
		dir := path.Dir(p)
		if err := mkdirAll(t, dir); err != nil {
			return err
		}
		return atomicWrite(t, p, content)
	})
}

func (b *Backend) StatBlob(ctx context.Context, t storage.BlobType, name string) (int64, error) {
	p := b.blobPath(ctx, t, name)
	var size int64
	err := b.withTarget(func(tgt *gonfs.Target) error {
		attr, err := tgt.Getattr(p)
		if err != nil {
			if isNotFound(err) && t == storage.BlobData && len(name) >= 2 {
				// Fallback: try unsharded path for pre-sharding data.
				up := b.blobPathUnsharded(ctx, name)
				attr, err = tgt.Getattr(up)
				if err != nil {
					if isNotFound(err) {
						return storage.ErrNotFound
					}
					return err
				}
				size = attr.Size()
				return nil
			}
			if isNotFound(err) {
				return storage.ErrNotFound
			}
			return err
		}
		size = attr.Size()
		return nil
	})
	return size, err
}

func (b *Backend) GetBlob(ctx context.Context, t storage.BlobType, name string, offset, length int64) (io.ReadCloser, error) {
	p := b.blobPath(ctx, t, name)
	var buf []byte
	err := b.withTarget(func(tgt *gonfs.Target) error {
		f, err := tgt.Open(p)
		if err != nil {
			if isNotFound(err) && t == storage.BlobData && len(name) >= 2 {
				// Fallback: try unsharded path for pre-sharding data.
				up := b.blobPathUnsharded(ctx, name)
				f, err = tgt.Open(up)
			}
			if err != nil {
				if isNotFound(err) {
					return storage.ErrNotFound
				}
				return err
			}
		}
		defer f.Close()

		if offset > 0 {
			if _, err := f.Seek(offset, io.SeekStart); err != nil {
				return err
			}
		}

		var reader io.Reader = f
		if length > 0 {
			reader = io.LimitReader(f, length)
		}
		buf, err = io.ReadAll(reader)
		return err
	})
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(buf)), nil
}

func (b *Backend) SaveBlob(ctx context.Context, t storage.BlobType, name string, data io.Reader) error {
	p := b.blobPath(ctx, t, name)
	content, err := io.ReadAll(data)
	if err != nil {
		return fmt.Errorf("read blob data: %w", err)
	}
	return b.withTarget(func(tgt *gonfs.Target) error {
		// Content-addressed: if blob already exists, skip.
		if _, err := tgt.Getattr(p); err == nil {
			return nil
		}
		// Also check unsharded path for pre-sharding data.
		if t == storage.BlobData && len(name) >= 2 {
			up := b.blobPathUnsharded(ctx, name)
			if _, err := tgt.Getattr(up); err == nil {
				return nil
			}
		}
		dir := path.Dir(p)
		if err := mkdirAll(tgt, dir); err != nil {
			return err
		}
		return atomicWrite(tgt, p, content)
	})
}

func (b *Backend) DeleteBlob(ctx context.Context, t storage.BlobType, name string) error {
	p := b.blobPath(ctx, t, name)
	return b.withTarget(func(tgt *gonfs.Target) error {
		if err := tgt.Remove(p); err != nil {
			if isNotFound(err) && t == storage.BlobData && len(name) >= 2 {
				// Fallback: try unsharded path for pre-sharding data.
				up := b.blobPathUnsharded(ctx, name)
				if err := tgt.Remove(up); err != nil {
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
	})
}

func (b *Backend) ListBlobs(ctx context.Context, t storage.BlobType) ([]storage.Blob, error) {
	dir := b.typePath(ctx, t)
	var blobs []storage.Blob
	err := b.withTarget(func(tgt *gonfs.Target) error {
		if t == storage.BlobData {
			return b.listShardedBlobs(tgt, dir, &blobs)
		}
		return b.listDir(tgt, dir, &blobs)
	})
	if err != nil {
		return nil, err
	}
	if blobs == nil {
		blobs = []storage.Blob{}
	}
	return blobs, nil
}

// listDir lists blobs in a single directory.
func (b *Backend) listDir(tgt *gonfs.Target, dir string, blobs *[]storage.Blob) error {
	entries, err := tgt.ReadDirPlus(dir)
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if name == "." || name == ".." {
			continue
		}
		if e.IsDir() {
			continue
		}
		*blobs = append(*blobs, storage.Blob{
			Name: name,
			Size: e.Size(),
		})
	}
	return nil
}

// listShardedBlobs iterates the 256 shard subdirectories and collects all blobs.
// It also collects any blobs stored directly in dataDir (pre-sharding fallback).
func (b *Backend) listShardedBlobs(tgt *gonfs.Target, dataDir string, blobs *[]storage.Blob) error {
	// Collect blobs from flat data/ directory (pre-sharding fallback).
	if err := b.listDir(tgt, dataDir, blobs); err != nil {
		return err
	}
	for i := 0; i < 256; i++ {
		subdir := path.Join(dataDir, fmt.Sprintf("%02x", i))
		if err := b.listDir(tgt, subdir, blobs); err != nil {
			return err
		}
	}
	return nil
}

// atomicWrite writes content to a temporary file and renames it to the final path.
func atomicWrite(t *gonfs.Target, finalPath string, content []byte) error {
	dir := path.Dir(finalPath)
	tmpPath := path.Join(dir, ".tmp-"+path.Base(finalPath))

	// Create the temp file.
	if _, err := t.Create(tmpPath, 0o644); err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	// Open and write.
	f, err := t.OpenFile(tmpPath, 0o644)
	if err != nil {
		t.Remove(tmpPath)
		return fmt.Errorf("open temp file: %w", err)
	}

	if _, err := f.Write(content); err != nil {
		f.Close()
		t.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := f.Close(); err != nil {
		t.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := t.Rename(tmpPath, finalPath); err != nil {
		t.Remove(tmpPath)
		return fmt.Errorf("rename temp to final: %w", err)
	}

	return nil
}

// mkdirAll creates all directories along the path, similar to os.MkdirAll.
func mkdirAll(t *gonfs.Target, dirPath string) error {
	if dirPath == "." || dirPath == "/" || dirPath == "" {
		return nil
	}

	// Check if it already exists.
	if attr, err := t.Getattr(dirPath); err == nil {
		if attr.IsDir() {
			return nil
		}
		return fmt.Errorf("%s exists but is not a directory", dirPath)
	}

	// Ensure parent exists.
	parent := path.Dir(dirPath)
	if parent != dirPath && parent != "." && parent != "/" {
		if err := mkdirAll(t, parent); err != nil {
			return err
		}
	}

	// Create directory.
	if _, err := t.Mkdir(dirPath, 0o755); err != nil {
		// Check if it was created concurrently.
		if attr, err2 := t.Getattr(dirPath); err2 == nil && attr.IsDir() {
			return nil
		}
		return err
	}
	return nil
}

// isNotFound checks whether err indicates a file-not-found condition.
// The go-nfs-client library maps NFS3ErrNoEnt to os.ErrNotExist.
func isNotFound(err error) bool {
	if os.IsNotExist(err) {
		return true
	}
	if nfsErr, ok := err.(*gonfs.Error); ok {
		return nfsErr.ErrorNum == gonfs.NFS3ErrNoEnt
	}
	s := err.Error()
	return strings.Contains(s, "NFS3ERR_NOENT") ||
		strings.Contains(s, "NFS3ERR_STALE")
}
