package filesystem

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c-mueller/ts-restic-server/internal/storage"
)

func TestNew_ResolvesBasePath(t *testing.T) {
	dir := t.TempDir()
	b, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if b.resolvedBase == "" {
		t.Error("resolvedBase should not be empty")
	}
	// resolvedBase must be an absolute path within or equal to dir
	resolved, _ := filepath.EvalSymlinks(dir)
	if b.resolvedBase != resolved {
		t.Errorf("resolvedBase = %q, want %q", b.resolvedBase, resolved)
	}
}

func TestValidatePath_NormalPath(t *testing.T) {
	dir := t.TempDir()
	b, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}

	// Create a file inside the base dir
	p := filepath.Join(dir, "testfile")
	os.WriteFile(p, []byte("data"), 0o600)

	// Existing file should validate
	if err := b.validatePath(p); err != nil {
		t.Errorf("validatePath for normal file: %v", err)
	}

	// Non-existent file in valid parent should validate
	if err := b.validatePath(filepath.Join(dir, "newfile")); err != nil {
		t.Errorf("validatePath for new file: %v", err)
	}
}

func TestValidatePath_SymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()

	b, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}

	// Create a symlink inside basePath pointing outside
	link := filepath.Join(dir, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	target := filepath.Join(link, "file.txt")
	// Create target so EvalSymlinks can resolve fully
	os.WriteFile(filepath.Join(outside, "file.txt"), []byte("secret"), 0o600)

	err = b.validatePath(target)
	if !errors.Is(err, ErrPathEscape) {
		t.Errorf("expected ErrPathEscape, got: %v", err)
	}
}

func TestValidatePath_SymlinkEscape_NewFile(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()

	b, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}

	// Create a symlink inside basePath pointing to an outside directory
	link := filepath.Join(dir, "escape-dir")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	// New file in symlinked directory should be detected
	err = b.validatePath(filepath.Join(link, "newfile"))
	if !errors.Is(err, ErrPathEscape) {
		t.Errorf("expected ErrPathEscape for new file in symlinked dir, got: %v", err)
	}
}

func TestIsSubPath(t *testing.T) {
	tests := []struct {
		path, base string
		want       bool
	}{
		{"/data", "/data", true},
		{"/data/repo", "/data", true},
		{"/data/repo/sub", "/data", true},
		{"/data2", "/data", false},
		{"/dat", "/data", false},
		{"/other", "/data", false},
	}
	for _, tc := range tests {
		if got := isSubPath(tc.path, tc.base); got != tc.want {
			t.Errorf("isSubPath(%q, %q) = %v, want %v", tc.path, tc.base, got, tc.want)
		}
	}
}

func TestSaveBlob_SkipsDuplicate(t *testing.T) {
	dir := t.TempDir()
	b, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	b.CreateRepo(ctx)

	name := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789" // pragma: allowlist secret
	original := []byte("blob content")

	// First write
	err = b.SaveBlob(ctx, storage.BlobData, name, bytes.NewReader(original))
	if err != nil {
		t.Fatalf("first save: %v", err)
	}

	// Second write with same name — should be skipped (no overwrite)
	different := []byte("different content that should be discarded")
	err = b.SaveBlob(ctx, storage.BlobData, name, bytes.NewReader(different))
	if err != nil {
		t.Fatalf("second save: %v", err)
	}

	// Verify original content is preserved
	rc, err := b.GetBlob(ctx, storage.BlobData, name, 0, 0)
	if err != nil {
		t.Fatalf("get blob: %v", err)
	}
	defer rc.Close()

	var buf bytes.Buffer
	buf.ReadFrom(rc)
	if !bytes.Equal(buf.Bytes(), original) {
		t.Errorf("blob content changed after duplicate write: got %q, want %q", buf.String(), string(original))
	}
}

func TestSaveBlob_WritesNewBlob(t *testing.T) {
	dir := t.TempDir()
	b, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	b.CreateRepo(ctx)

	name := "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef" // pragma: allowlist secret
	data := []byte("test data")

	err = b.SaveBlob(ctx, storage.BlobData, name, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("save blob: %v", err)
	}

	size, err := b.StatBlob(ctx, storage.BlobData, name)
	if err != nil {
		t.Fatalf("stat blob: %v", err)
	}
	if size != int64(len(data)) {
		t.Errorf("blob size = %d, want %d", size, len(data))
	}
}

func TestSaveBlobSymlinkBlocked(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()

	b, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	b.CreateRepo(ctx)

	// Replace the "data" directory with a symlink to outside
	dataDir := filepath.Join(dir, "data")
	os.RemoveAll(dataDir)
	if err := os.Symlink(outside, dataDir); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	name := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789" // pragma: allowlist secret
	err = b.SaveBlob(ctx, storage.BlobData, name, strings.NewReader("data"))
	if !errors.Is(err, ErrPathEscape) {
		t.Errorf("expected ErrPathEscape, got: %v", err)
	}
}

func TestGetBlobSymlinkBlocked(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()

	b, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	b.CreateRepo(ctx)

	// Create a file outside, then symlink to it from inside
	outsideFile := filepath.Join(outside, "secret")
	os.WriteFile(outsideFile, []byte("secret data"), 0o600)

	keysDir := filepath.Join(dir, "keys")
	link := filepath.Join(keysDir, "abcdef01")
	os.Symlink(outsideFile, link)

	_, err = b.GetBlob(ctx, storage.BlobKeys, "abcdef01", 0, 0)
	if !errors.Is(err, ErrPathEscape) {
		t.Errorf("expected ErrPathEscape for symlinked blob, got: %v", err)
	}
}
