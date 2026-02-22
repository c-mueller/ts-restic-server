# ts-restic-server

A Restic REST server in Go implementing the official REST backend API (v1 + v2) with pluggable storage backends and optional Tailscale listener.

## Build & Run

```bash
go build -o ts-restic-server .
./ts-restic-server serve
./ts-restic-server serve --storage-backend memory   # in-memory backend
./ts-restic-server serve --config config.yaml       # custom config
```

## Project Structure

- `main.go` — entry point, calls `cmd.Execute()`
- `cmd/` — Cobra commands (root, serve)
- `internal/config/` — Config structs, defaults, validation
- `internal/server/` — Server + listener factory (plain TCP / tsnet)
- `internal/api/` — Echo routes, handlers, version negotiation
- `internal/middleware/` — Request ID, structured logging, panic recovery
- `internal/storage/` — Backend interface + implementations (memory, filesystem, s3)

## Configuration

Priority: CLI flags > config file > environment variables (prefix `RESTIC_`).

## Testing

```bash
# Start with memory backend
go run . serve --storage-backend memory

# Test with curl
curl -X POST http://localhost:8880/?create=true
curl -X POST --data-binary @testfile http://localhost:8880/config
curl http://localhost:8880/config

# Test with restic
restic -r rest:http://localhost:8880/ init
```

## Key Design Decisions

- Append-only mode: DELETE returns 403, except lock deletion stays allowed
- Memory backend: 100MB cap, ErrQuotaExceeded on overflow
- Filesystem backend: atomic writes with fsync, data/00-ff subdirectories
- S3 backend: uses aws-sdk-go-v2, supports custom endpoints (MinIO etc.)
- No auth in v1: Tailscale provides identity; ACLs are a future feature
