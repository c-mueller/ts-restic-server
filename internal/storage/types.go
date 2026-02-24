package storage

import "errors"

type BlobType string

const (
	BlobData      BlobType = "data"
	BlobKeys      BlobType = "keys"
	BlobLocks     BlobType = "locks"
	BlobSnapshots BlobType = "snapshots"
	BlobIndex     BlobType = "index"
)

var ValidBlobTypes = map[BlobType]bool{
	BlobData:      true,
	BlobKeys:      true,
	BlobLocks:     true,
	BlobSnapshots: true,
	BlobIndex:     true,
}

type Blob struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
	ErrQuotaExceeded = errors.New("quota exceeded")
	ErrRepoNotFound  = errors.New("repository not found")
	ErrRepoExists    = errors.New("repository already exists")
)
