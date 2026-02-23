package rclone

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/c-mueller/ts-restic-server/internal/middleware"
	"github.com/c-mueller/ts-restic-server/internal/storage"
)

type Backend struct {
	client   *http.Client
	endpoint string
	username string
	password string
}

func New(endpoint, username, password string) *Backend {
	return &Backend{
		client:   &http.Client{},
		endpoint: strings.TrimSuffix(endpoint, "/"),
		username: username,
		password: password,
	}
}

func (b *Backend) CreateRepo(ctx context.Context) error {
	url := b.repoURL(ctx) + "?create=true"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	b.setAuth(req)

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return storage.ErrRepoExists
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("create repo: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (b *Backend) DeleteRepo(ctx context.Context) error {
	url := b.repoURL(ctx)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	b.setAuth(req)

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return storage.ErrRepoNotFound
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("delete repo: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (b *Backend) StatConfig(ctx context.Context) (int64, error) {
	url := b.repoURL(ctx) + "config"
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return 0, err
	}
	b.setAuth(req)

	resp, err := b.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return 0, storage.ErrNotFound
	}
	if resp.StatusCode >= 300 {
		return 0, fmt.Errorf("stat config: unexpected status %d", resp.StatusCode)
	}
	return resp.ContentLength, nil
}

func (b *Backend) GetConfig(ctx context.Context) (io.ReadCloser, error) {
	url := b.repoURL(ctx) + "config"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	b.setAuth(req)

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, storage.ErrNotFound
	}
	if resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("get config: unexpected status %d", resp.StatusCode)
	}
	return resp.Body, nil
}

func (b *Backend) SaveConfig(ctx context.Context, data io.Reader) error {
	url := b.repoURL(ctx) + "config"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, data)
	if err != nil {
		return err
	}
	b.setAuth(req)

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("save config: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (b *Backend) StatBlob(ctx context.Context, t storage.BlobType, name string) (int64, error) {
	url := b.blobURL(ctx, t, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return 0, err
	}
	b.setAuth(req)

	resp, err := b.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return 0, storage.ErrNotFound
	}
	if resp.StatusCode >= 300 {
		return 0, fmt.Errorf("stat blob: unexpected status %d", resp.StatusCode)
	}
	return resp.ContentLength, nil
}

func (b *Backend) GetBlob(ctx context.Context, t storage.BlobType, name string, offset, length int64) (io.ReadCloser, error) {
	url := b.blobURL(ctx, t, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	b.setAuth(req)

	if offset > 0 || length > 0 {
		var rangeVal string
		if length > 0 {
			rangeVal = fmt.Sprintf("bytes=%d-%d", offset, offset+length-1)
		} else {
			rangeVal = fmt.Sprintf("bytes=%d-", offset)
		}
		req.Header.Set("Range", rangeVal)
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, storage.ErrNotFound
	}
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return nil, fmt.Errorf("get blob: unexpected status %d", resp.StatusCode)
	}
	return resp.Body, nil
}

func (b *Backend) SaveBlob(ctx context.Context, t storage.BlobType, name string, data io.Reader) error {
	url := b.blobURL(ctx, t, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, data)
	if err != nil {
		return err
	}
	b.setAuth(req)

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("save blob: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (b *Backend) DeleteBlob(ctx context.Context, t storage.BlobType, name string) error {
	url := b.blobURL(ctx, t, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	b.setAuth(req)

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return storage.ErrNotFound
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("delete blob: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (b *Backend) ListBlobs(ctx context.Context, t storage.BlobType) ([]storage.Blob, error) {
	url := b.repoURL(ctx) + string(t) + "/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	b.setAuth(req)
	req.Header.Set("Accept", "application/vnd.x.restic.rest.v2")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return []storage.Blob{}, nil
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("list blobs: unexpected status %d", resp.StatusCode)
	}

	var blobs []storage.Blob
	if err := json.NewDecoder(resp.Body).Decode(&blobs); err != nil {
		return nil, fmt.Errorf("list blobs: decoding response: %w", err)
	}
	if blobs == nil {
		blobs = []storage.Blob{}
	}
	return blobs, nil
}

func (b *Backend) setAuth(req *http.Request) {
	if b.username != "" || b.password != "" {
		req.SetBasicAuth(b.username, b.password)
	}
}

// repoURL returns the base URL for repo-level operations, including the repo
// prefix extracted from the request context. The returned URL always ends with "/".
func (b *Backend) repoURL(ctx context.Context) string {
	repo := middleware.GetRepoPrefix(ctx)
	if repo != "" {
		return b.endpoint + "/" + repo + "/"
	}
	return b.endpoint + "/"
}

// blobURL returns the full URL for a specific blob.
func (b *Backend) blobURL(ctx context.Context, t storage.BlobType, name string) string {
	return b.repoURL(ctx) + string(t) + "/" + name
}
