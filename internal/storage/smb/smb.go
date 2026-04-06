package smb

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/c-mueller/ts-restic-server/internal/middleware"
	"github.com/c-mueller/ts-restic-server/internal/storage"
	"github.com/hirochachacha/go-smb2"
)

// Backend implements storage.Backend using an SMB2/3 share.
// It uses a pure-Go SMB client (go-smb2) and does not require
// OS-level mounting of the share.
type Backend struct {
	server   string
	port     int
	share    string
	username string
	password string
	domain   string
	basePath string

	mu       sync.Mutex
	conn     net.Conn
	session  *smb2.Session
	smbShare *smb2.Share
}

// New creates and connects a new SMB backend.
func New(server string, port int, share, username, password, domain, basePath string) (*Backend, error) {
	b := &Backend{
		server:   server,
		port:     port,
		share:    share,
		username: username,
		password: password,
		domain:   domain,
		basePath: basePath,
	}
	if err := b.connect(); err != nil {
		return nil, fmt.Errorf("smb connect: %w", err)
	}
	return b, nil
}

func (b *Backend) connect() error {
	addr := fmt.Sprintf("%s:%d", b.server, b.port)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}

	d := &smb2.Dialer{
		Initiator: &smb2.NTLMInitiator{
			User:     b.username,
			Password: b.password,
			Domain:   b.domain,
		},
	}

	session, err := d.Dial(conn)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smb dial: %w", err)
	}

	share, err := session.Mount(b.share)
	if err != nil {
		session.Logoff()
		conn.Close()
		return fmt.Errorf("mount share %q: %w", b.share, err)
	}

	b.conn = conn
	b.session = session
	b.smbShare = share
	return nil
}

// ensureConnected reconnects if the SMB session has been lost.
func (b *Backend) ensureConnected() error {
	if b.smbShare != nil {
		// Quick health check: stat the base path (or root).
		target := b.basePath
		if target == "" {
			target = "."
		}
		if _, err := b.smbShare.Stat(target); err == nil {
			return nil
		}
	}
	b.disconnect()
	return b.connect()
}

func (b *Backend) disconnect() {
	if b.smbShare != nil {
		b.smbShare.Umount()
		b.smbShare = nil
	}
	if b.session != nil {
		b.session.Logoff()
		b.session = nil
	}
	if b.conn != nil {
		b.conn.Close()
		b.conn = nil
	}
}

// withShare acquires the mutex, ensures the connection is live, and calls fn
// with the connected share. All backend methods use this to serialize access
// and handle reconnection transparently.
func (b *Backend) withShare(fn func(s *smb2.Share) error) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.ensureConnected(); err != nil {
		return err
	}
	return fn(b.smbShare)
}

// Close releases the SMB connection. It should be called when the backend
// is no longer needed.
func (b *Backend) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.disconnect()
}

// repoPath returns the base directory for a repository, derived from
// the repo prefix in the context and the configured base path.
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
	return path.Join(b.repoPath(ctx), string(t), name)
}

func (b *Backend) CreateRepo(ctx context.Context) error {
	rp := b.repoPath(ctx)
	dirs := []string{
		rp,
		path.Join(rp, "keys"),
		path.Join(rp, "locks"),
		path.Join(rp, "snapshots"),
		path.Join(rp, "index"),
		path.Join(rp, "data"),
	}

	return b.withShare(func(s *smb2.Share) error {
		for _, dir := range dirs {
			if err := s.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("create directory %s: %w", dir, err)
			}
		}
		return nil
	})
}

func (b *Backend) DeleteRepo(ctx context.Context) error {
	rp := b.repoPath(ctx)
	return b.withShare(func(s *smb2.Share) error {
		if _, err := s.Stat(rp); err != nil {
			if isNotFound(err) {
				return storage.ErrRepoNotFound
			}
			return err
		}
		return s.RemoveAll(rp)
	})
}

func (b *Backend) StatConfig(ctx context.Context) (int64, error) {
	p := b.configPath(ctx)
	var size int64
	err := b.withShare(func(s *smb2.Share) error {
		info, err := s.Stat(p)
		if err != nil {
			if isNotFound(err) {
				return storage.ErrNotFound
			}
			return err
		}
		size = info.Size()
		return nil
	})
	return size, err
}

func (b *Backend) GetConfig(ctx context.Context) (io.ReadCloser, error) {
	p := b.configPath(ctx)
	// Read the entire config into memory so we don't hold the mutex.
	var buf []byte
	err := b.withShare(func(s *smb2.Share) error {
		f, err := s.Open(p)
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
	return b.withShare(func(s *smb2.Share) error {
		dir := path.Dir(p)
		if err := s.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		return atomicWrite(s, p, content)
	})
}

func (b *Backend) StatBlob(ctx context.Context, t storage.BlobType, name string) (int64, error) {
	p := b.blobPath(ctx, t, name)
	var size int64
	err := b.withShare(func(s *smb2.Share) error {
		info, err := s.Stat(p)
		if err != nil {
			if isNotFound(err) {
				return storage.ErrNotFound
			}
			return err
		}
		size = info.Size()
		return nil
	})
	return size, err
}

func (b *Backend) GetBlob(ctx context.Context, t storage.BlobType, name string, offset, length int64) (io.ReadCloser, error) {
	p := b.blobPath(ctx, t, name)
	// Read the blob (or portion) into memory so we don't hold the mutex
	// across the caller's read lifecycle.
	var buf []byte
	err := b.withShare(func(s *smb2.Share) error {
		f, err := s.Open(p)
		if err != nil {
			if isNotFound(err) {
				return storage.ErrNotFound
			}
			return err
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
	return b.withShare(func(s *smb2.Share) error {
		// Content-addressed: if blob already exists, skip.
		if _, err := s.Stat(p); err == nil {
			return nil
		}
		dir := path.Dir(p)
		if err := s.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		return atomicWrite(s, p, content)
	})
}

func (b *Backend) DeleteBlob(ctx context.Context, t storage.BlobType, name string) error {
	p := b.blobPath(ctx, t, name)
	return b.withShare(func(s *smb2.Share) error {
		if err := s.Remove(p); err != nil {
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
	err := b.withShare(func(s *smb2.Share) error {
		entries, err := s.ReadDir(dir)
		if err != nil {
			if isNotFound(err) {
				blobs = []storage.Blob{}
				return nil
			}
			return err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			blobs = append(blobs, storage.Blob{
				Name: e.Name(),
				Size: e.Size(),
			})
		}
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

// atomicWrite writes content to a temporary file on the share and renames
// it to the final path. This provides atomic-write semantics on SMB.
func atomicWrite(s *smb2.Share, finalPath string, content []byte) error {
	dir := path.Dir(finalPath)
	tmpPath := path.Join(dir, ".tmp-"+path.Base(finalPath))

	f, err := s.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	if _, err := f.Write(content); err != nil {
		f.Close()
		s.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := f.Close(); err != nil {
		s.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := s.Rename(tmpPath, finalPath); err != nil {
		s.Remove(tmpPath)
		return fmt.Errorf("rename temp to final: %w", err)
	}

	return nil
}

// isNotFound checks whether err indicates a file-not-found condition.
func isNotFound(err error) bool {
	if os.IsNotExist(err) {
		return true
	}
	s := err.Error()
	return strings.Contains(s, "not found") ||
		strings.Contains(s, "does not exist") ||
		strings.Contains(s, "STATUS_OBJECT_NAME_NOT_FOUND") ||
		strings.Contains(s, "STATUS_OBJECT_PATH_NOT_FOUND")
}
