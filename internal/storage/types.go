package storage

import (
	"errors"
	"regexp"
)

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

var validBlobNameRe = regexp.MustCompile(`^[0-9a-fA-F]+$`)

// ValidBlobName reports whether name is a non-empty hex string. Restic names
// every blob by its SHA-256, so anything else is not a blob name, and
// rejecting it keeps a name from reaching outside its type's directory.
func ValidBlobName(name string) bool {
	return validBlobNameRe.MatchString(name)
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
