# ts-restic-server

A [restic](https://restic.net/) REST server written in Go, implementing the official REST backend API (v1 + v2). It features pluggable storage backends and an optional [Tailscale](https://tailscale.com/) listener for zero-config, encrypted connectivity via [tsnet](https://pkg.go.dev/tailscale.com/tsnet).

## Features

- Full restic REST API (v1 and v2) compatibility
- Multiple storage backends: Filesystem, S3-compatible, In-Memory
- Multi-repository support via URL path prefixes (e.g. `/host-a/backups`, `/host-b/docs`)
- Optional Tailscale integration for TLS without certificates or port forwarding
- Append-only mode (deletes blocked except for lock removal)
- Structured JSON logging with per-request IDs (zap)
- Configuration via CLI flags, config file, or environment variables

## Early Stage Notice

This project is in early development and was largely vibe-coded with AI assistance. It may contain bugs or missing edge cases. There are no Dockerfiles, systemd units, or packages provided yet. **Pull requests and bug reports are welcome!**

## Building

Requires Go 1.23+.

```bash
go build -o ts-restic-server .
```

## Usage

```bash
# Start with filesystem backend (default)
./ts-restic-server serve

# Start with in-memory backend
./ts-restic-server serve --storage-backend memory

# Start with a config file
./ts-restic-server serve --config config.yaml

# Start with Tailscale listener
./ts-restic-server serve --listen-mode tailscale
```

### Using with restic

```bash
# Initialize a repository
restic -r rest:http://localhost:8880/ init

# Initialize a repository under a sub-path (multi-repo)
restic -r rest:http://localhost:8880/my-host/backups init

# Backup
restic -r rest:http://localhost:8880/my-host/backups backup ~/Documents

# With Tailscale
restic -r rest:https://my-restic-server.my-tailnet.ts.net/my-host/backups init
```

## Configuration

Configuration is loaded with the following priority: **CLI flags > config file > environment variables**.

Environment variables use the prefix `RESTIC_` with underscores replacing dots (e.g. `RESTIC_STORAGE_BACKEND=s3`).

See [`config.example.yaml`](config.example.yaml) for all available options:

```yaml
listen: ":8880"
listen_mode: plain       # "plain" or "tailscale"
append_only: false
log_level: info

tailscale:
  hostname: restic-server
  state_dir: ./ts-state
  auth_key: ""

storage:
  backend: filesystem     # "filesystem", "s3", "memory"
  path: ./restic_data
  max_memory_bytes: 104857600  # 100MB for memory backend
  s3:
    bucket: my-bucket
    prefix: ""
    region: eu-central-1
    endpoint: ""
    access_key: ""
    secret_key: ""
```

### CLI Flags

| Flag | Description |
|------|-------------|
| `--config` | Path to config file (default: `./config.yaml`) |
| `--listen` | Listen address (default: `:8880`) |
| `--listen-mode` | `plain` or `tailscale` |
| `--append-only` | Enable append-only mode |
| `--log-level` | `debug`, `info`, `warn`, `error` |
| `--storage-backend` | `filesystem`, `s3`, `memory` |
| `--storage-path` | Path for filesystem backend |

## Storage Backends

### Filesystem

The default backend. Stores data in the local filesystem with restic's standard directory layout (data/00-ff subdirectories, atomic writes with fsync).

```yaml
storage:
  backend: filesystem
  path: ./restic_data
```

### S3-Compatible

Works with AWS S3, MinIO, Hetzner Object Storage, and other S3-compatible providers. Supports custom endpoints and static credentials. If `access_key` and `secret_key` are left empty, the standard AWS credential chain is used (environment, shared credentials file, IAM role, etc.).

```yaml
storage:
  backend: s3
  s3:
    bucket: my-backup-bucket
    prefix: ""                                    # optional key prefix
    region: eu-central-1
    endpoint: https://fsn1.your-objectstorage.com # leave empty for AWS
    access_key: AKIA...
    secret_key: wJal...
```

### In-Memory

Useful for testing. All data is lost when the server stops. Enforces a configurable memory cap (default 100MB).

```yaml
storage:
  backend: memory
  max_memory_bytes: 104857600
```

## Tailscale Integration

When `listen_mode` is set to `tailscale`, the server uses [tsnet](https://pkg.go.dev/tailscale.com/tsnet) to join your Tailnet and serve over HTTPS with automatic TLS certificates. No port forwarding or manual certificate management required.

```yaml
listen_mode: tailscale
tailscale:
  hostname: restic-server        # appears as restic-server.my-tailnet.ts.net
  state_dir: ./ts-state          # persistent Tailscale state
  auth_key: tskey-auth-...       # optional, for headless auth
```

The Tailscale listener always binds to port 443, so restic clients can connect without specifying a port.

## Multi-Repository Support

The server supports hosting multiple independent repositories under different URL paths. The path prefix is transparently passed to the storage backend:

- **S3**: path prefix becomes part of the S3 key (e.g. `{prefix}/host-a/backups/data/...`)
- **Filesystem**: path prefix becomes a subdirectory (e.g. `./restic_data/host-a/backups/data/...`)
- **Memory**: each path prefix gets its own isolated in-memory store

```bash
restic -r rest:http://localhost:8880/host-a/daily init
restic -r rest:http://localhost:8880/host-b/daily init
# These are completely independent repositories
```

## License

Apache License 2.0 - see [LICENSE](LICENSE).
