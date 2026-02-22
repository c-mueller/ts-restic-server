package storage

import (
	"context"
	"io"
)

type Backend interface {
	CreateRepo(ctx context.Context) error
	DeleteRepo(ctx context.Context) error

	StatConfig(ctx context.Context) (int64, error)
	GetConfig(ctx context.Context) (io.ReadCloser, error)
	SaveConfig(ctx context.Context, data io.Reader) error

	StatBlob(ctx context.Context, t BlobType, name string) (int64, error)
	GetBlob(ctx context.Context, t BlobType, name string, offset, length int64) (io.ReadCloser, error)
	SaveBlob(ctx context.Context, t BlobType, name string, data io.Reader) error
	DeleteBlob(ctx context.Context, t BlobType, name string) error
	ListBlobs(ctx context.Context, t BlobType) ([]Blob, error)
}
