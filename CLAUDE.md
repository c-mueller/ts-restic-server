# ts-restic-server

Restic REST server (v1 + v2 API) in Go with pluggable storage backends and optional Tailscale listener.

## Build & Run

```bash
go build -o ts-restic-server .
./ts-restic-server serve                            # filesystem backend, :8880
./ts-restic-server serve --storage-backend memory   # in-memory backend
./ts-restic-server serve --listen-mode tailscale    # Tailscale TLS on :443
./ts-restic-server serve --config config.yaml       # custom config
```

## Project Structure

- `main.go` — entry point, calls `cmd.Execute()`
- `cmd/root.go` — Cobra root command + Viper config init
- `cmd/serve.go` — `serve` command: wires config, backend, logger, server
- `internal/config/` — Config structs, defaults, validation
- `internal/server/server.go` — Server struct: Echo + logger + backend, Run/Shutdown
- `internal/server/listener.go` — Listener factory: plain TCP vs tsnet (TLS on :443)
- `internal/api/router.go` — Registers all Echo routes + middleware
- `internal/api/handler.go` — Handler struct (backend + logger refs)
- `internal/api/repo.go` — POST /?create=true, DELETE / (repo management)
- `internal/api/config_handler.go` — HEAD/GET/POST /config
- `internal/api/blob.go` — HEAD/GET/POST/DELETE /:type/:name
- `internal/api/list.go` — GET /:type/ (v1: string[], v2: {name,size}[])
- `internal/api/version.go` — API version negotiation (Accept header)
- `internal/middleware/requestid.go` — UUID per request, X-Request-ID header
- `internal/middleware/logger.go` — Zap structured request logging
- `internal/middleware/recover.go` — Panic recovery
- `internal/middleware/repoprefix.go` — Extracts repo path prefix, rewrites URL for routing
- `internal/storage/backend.go` — Backend interface
- `internal/storage/types.go` — BlobType, Blob struct, sentinel errors
- `internal/storage/memory/` — In-memory backend (configurable cap, sync.RWMutex)
- `internal/storage/filesystem/` — Filesystem backend (atomic writes, fsync, data/00-ff)
- `internal/storage/s3/` — S3 backend (aws-sdk-go-v2, custom endpoints, static creds)
- `internal/storage/webdav/` — WebDAV backend (gowebdav, Nextcloud/ownCloud/HiDrive/Box)

## Configuration

Priority: CLI flags > config file (`--config`) > env vars (prefix `RESTIC_`, e.g. `RESTIC_STORAGE_BACKEND`).

See `config.example.yaml` for all options.

## Key Design Decisions

- **Multi-repo**: URL path prefix (e.g. `/host/backup`) scopes storage per repo
- **Append-only**: DELETE on blobs returns 403, lock deletion stays allowed
- **Memory backend**: shared quota across all repos, ErrQuotaExceeded on overflow
- **Filesystem**: atomic writes (temp + fsync + rename), optional data/00-ff sharding
- **S3**: supports custom endpoints (MinIO, Hetzner, etc.), static or chain credentials
- **WebDAV**: gowebdav client, flat structure per type (no data/00-ff sharding), Basic Auth
- **Tailscale**: tsnet ListenTLS on :443, state_dir for persistent keys
- **No auth in v1**: Tailscale provides identity; ACLs are a future feature

## Testing

```bash
# Quick smoke test with memory backend
go run . serve --storage-backend memory --listen-mode plain
RESTIC_PASSWORD=test restic -r rest:http://localhost:8880/test init
RESTIC_PASSWORD=test restic -r rest:http://localhost:8880/test backup .
RESTIC_PASSWORD=test restic -r rest:http://localhost:8880/test snapshots
```
