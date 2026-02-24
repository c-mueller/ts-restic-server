# Testing

## Prerequisites

| Prerequisite | Required for |
|---|---|
| Go (see `go.mod`) | All tests |
| `restic` binary | Integration tests |
| Docker | S3 backend test (MinIO container) |

## Running tests

```bash
# All tests (unit + integration)
go test ./...

# Unit tests only (skips integration tests)
go test -short ./...

# Integration tests only
go test -v ./tests/integration/

# Single backend
go test -v -run TestMemoryBackend      ./tests/integration/
go test -v -run TestFilesystemBackend  ./tests/integration/
go test -v -run TestWebDAVBackend      ./tests/integration/
go test -v -run TestRcloneBackend      ./tests/integration/
go test -v -run TestS3Backend          ./tests/integration/
```

## Integration tests

The integration tests exercise the full restic lifecycle against each storage backend:

1. Generate test data (deterministic random data, seed-based)
2. `restic init`
3. `restic backup` (snapshot 1)
4. Add delta files, `restic backup` (snapshot 2)
5. Verify snapshot list (expected: 2)
6. `restic restore` snapshot 1 + SHA-256 hash comparison
7. `restic restore` snapshot 2 + SHA-256 hash comparison
8. `restic forget --prune` snapshot 1
9. Verify snapshot list (expected: 1)

### Test data

**Dataset A (Mixed-Size, ~100 MB):**

- 50 × 1 MB files
- 100 × 100 KB files
- 200 × 10 KB files
- 10 × 512 KB delta files (for snapshot 2)

**Dataset B (Single Large File, 100 MB):**

- 1 × 100 MB file (edge case for pack handling)

All data is poorly compressible (random data) and deterministic (fixed seed).

### Backend test matrix

| Backend | External dependency | Infrastructure |
|---|---|---|
| Memory | None | In-process |
| Filesystem | None | `t.TempDir()` |
| WebDAV | None | In-process Go WebDAV server |
| Rclone | None | Second ts-restic-server instance with memory backend |
| S3 | Docker | MinIO via testcontainers-go |

### Skip behavior

Tests are automatically skipped when prerequisites are missing:

| Condition | Effect |
|---|---|
| `-short` flag | All integration tests skipped |
| `restic` binary missing | All integration tests skipped |
| Docker not available | Only S3 test skipped |

## CI pipeline

The GitHub Actions pipeline consists of three stages:

1. **Lint** — pre-commit hooks (gofmt, markdownlint, yamllint, detect-secrets, trailing whitespace, YAML syntax)
2. **Unit & Vet** — `go vet` + `go test -short` (fast, no restic/Docker required)
3. **Integration** — One job per backend (parallel, matrix build), runs only when unit tests pass

The S3 tests use an ephemeral MinIO container via testcontainers-go. Docker is pre-installed on the GitHub Actions runners.
