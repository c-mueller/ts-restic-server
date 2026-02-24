# ts-restic-server

A [restic](https://restic.net/) REST server written in Go, implementing the official REST backend API (v1 + v2). It features pluggable storage backends and an optional [Tailscale](https://tailscale.com/) listener for zero-config, encrypted connectivity via [tsnet](https://pkg.go.dev/tailscale.com/tsnet).

## Features

- Full restic REST API (v1 and v2) compatibility
- Multiple storage backends: Filesystem, S3-compatible, WebDAV, Rclone, In-Memory
- Multi-repository support via URL path prefixes (e.g. `/host-a/backups`, `/host-b/docs`)
- Optional Tailscale integration for TLS without certificates or port forwarding
- ACL engine with per-identity, per-repo-path access control (Tailscale tags, users, hostnames, IPs)
- Append-only mode (deletes blocked except for lock removal)
- Structured JSON logging with per-request IDs (zap)
- Configuration via CLI flags, config file, or environment variables

## Early Stage Notice

This project is in early development and was largely vibe-coded with AI assistance. It may contain bugs or missing edge cases. **Pull requests and bug reports are welcome!**

## Docker

```bash
docker pull ghcr.io/c-mueller/ts-restic-server:latest
```

```bash
docker run -d \
  -p 8880:8880 \
  -v ./config.yaml:/etc/ts-restic-server/config.yaml:ro \
  -v restic-data:/data \
  ghcr.io/c-mueller/ts-restic-server:latest \
  serve --config /etc/ts-restic-server/config.yaml
```

Or with Docker Compose — create a directory with `compose.yaml` and `config.yaml`:

```yaml
# compose.yaml
services:
  ts-restic-server:
    image: ghcr.io/c-mueller/ts-restic-server:latest
    ports:
      - "8880:8880"
    volumes:
      - ./config.yaml:/etc/ts-restic-server/config.yaml:ro
      - data:/data
    command: ["serve", "--config", "/etc/ts-restic-server/config.yaml"]
    restart: unless-stopped

volumes:
  data:
```

```yaml
# config.yaml
listen: ":8880"
listen_mode: plain
storage:
  backend: filesystem
  path: /data
  data_sharding: true
```

```bash
docker compose up -d
```

Multi-arch images (amd64 + arm64) are published to `ghcr.io/c-mueller/ts-restic-server` on every push to master. Tagged releases are available under the corresponding tag name.

See [docs/docker.md](docs/docker.md) for more details.

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
  backend: filesystem     # "filesystem", "s3", "webdav", "rclone", "memory"
  path: ./restic_data
  max_memory_bytes: 104857600  # 100MB for memory backend
  data_sharding: true          # split data/ into 256 subdirs (00-ff); for filesystem and webdav
  s3:
    bucket: my-bucket
    prefix: ""
    region: eu-central-1
    endpoint: ""
    access_key: ""
    secret_key: ""
  webdav:
    endpoint: ""
    username: ""
    password: ""
    prefix: ""
```

### CLI Flags

| Flag | Description |
|------|-------------|
| `--config` | Path to config file (default: `./config.yaml`) |
| `--listen` | Listen address (default: `:8880`) |
| `--listen-mode` | `plain` or `tailscale` |
| `--append-only` | Enable append-only mode |
| `--log-level` | `debug`, `info`, `warn`, `error` |
| `--storage-backend` | `filesystem`, `s3`, `webdav`, `rclone`, `memory` |
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

### WebDAV

Works with any WebDAV-compatible cloud storage: Nextcloud, ownCloud, HiDrive, Box, and others. No rclone intermediary needed.

```yaml
storage:
  backend: webdav
  webdav:
    endpoint: https://cloud.example.com/remote.php/dav/files/user
    username: myuser
    password: mypassword
    prefix: backups            # optional subdirectory within the WebDAV server
```

### Rclone

Proxies all storage operations to a remote restic REST server, such as [`rclone serve restic`](https://rclone.org/commands/rclone_serve_restic/). This enables using any of rclone's 70+ supported cloud providers as storage.

```yaml
storage:
  backend: rclone
  rclone:
    endpoint: http://localhost:8080
    username: ""       # optional basic auth
    password: ""
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
- **WebDAV**: path prefix becomes a subdirectory on the WebDAV server (e.g. `{prefix}/host-a/backups/data/...`)
- **Filesystem**: path prefix becomes a subdirectory (e.g. `./restic_data/host-a/backups/data/...`)
- **Memory**: each path prefix gets its own isolated in-memory store

```bash
restic -r rest:http://localhost:8880/host-a/daily init
restic -r rest:http://localhost:8880/host-b/daily init
# These are completely independent repositories
```

## FAQ

### Can I run this without Tailscale?

Yes. Set `listen_mode: plain` (the default) and the server listens on a regular TCP port. However, plain mode serves unencrypted HTTP. For production use without Tailscale, you should place the server behind a reverse proxy (e.g. nginx, Caddy, Traefik) that handles TLS termination. The server itself does not support standalone TLS certificates — it's either Tailscale-managed TLS or plain HTTP.

Example with Caddy:

```text
restic.example.com {
    reverse_proxy localhost:8880
}
```

### What storage backends are available?

- **Filesystem** — local disk storage with restic's standard directory layout. Default and simplest option.
- **S3-compatible** — any S3-compatible object storage. Supports custom endpoints for non-AWS providers.
- **WebDAV** — any WebDAV-compatible cloud storage (Nextcloud, ownCloud, HiDrive, Box, etc.).
- **Rclone** — proxies to `rclone serve restic` or any restic REST server. Access 70+ cloud providers via rclone.
- **In-Memory** — ephemeral storage for testing. Data is lost on restart. Configurable memory cap.

### Which S3 providers have been tested?

So far only **Hetzner Object Storage** has been tested. AWS S3 and MinIO should work but have not been verified yet. If you successfully use another provider, please open an issue or PR to help expand this list.

### Does it support authentication / access control?

Yes. The built-in ACL engine provides fine-grained access control per identity and repository path. In Tailscale mode, identities are resolved via the WhoIs API — giving you access to Tailscale tags (`tag:backup`), user logins (`alice@example.com`), hostnames, and IPs. In plain mode, identities are resolved via rDNS.

```yaml
acl:
  default_role: deny
  rules:
    - paths: ["/"]
      identities: ["tag:backup"]
      permission: full-access
    - paths: ["/alice"]
      identities: ["alice@example.com"]
      permission: full-access
```

See [docs/acl.md](docs/acl.md) for full documentation including cascading rules, permissions, trusted proxies, and examples.

### Can I host multiple repositories on one server?

Yes. Every URL path prefix creates an independent repository. For example, `/server-a/daily` and `/server-b/weekly` are completely isolated from each other, even though they share the same storage backend (same S3 bucket, same filesystem root, etc.). This works identically to the official restic REST server.

### How does append-only mode work?

With `append_only: true`, the server rejects all DELETE requests on blobs with HTTP 403, preventing data from being removed. Lock deletion remains allowed so that stale locks can still be cleaned up. This is useful as a safeguard against accidental or malicious data deletion.

## Disclaimer

This project is not affiliated with or endorsed by the [restic](https://restic.net/) project or [Tailscale Inc.](https://tailscale.com/) Tailscale is a registered trademark of Tailscale Inc.

## License

Apache License 2.0 - see [LICENSE](LICENSE).
