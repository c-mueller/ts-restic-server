# Ralph Fix Plan

---

## Epic 1: Network Storage Backends (CIFS/SMB + NFS)

### Task 1.1: CIFS/SMB Storage Backend

**Priority:** HIGH
**Type:** Feature

**Description:**
Implement a CIFS/SMB (Samba) storage backend that allows the restic server to use a remote SMB share as storage without mounting it via the OS kernel. The backend should act as a proxy: the server connects to the SMB share using a Go-native or CGO-based SMB client library, reads/writes blobs directly over the SMB protocol, and exposes them through the existing `storage.Backend` interface.

**Implementation approach (in order of preference):**

1. **Go-native SMB client** (no CGO): Use a pure-Go SMB2/3 library (e.g. `github.com/hirochachacha/go-smb2`) to connect to the share and perform file I/O directly over the SMB protocol. This is the preferred approach for cross-platform compatibility and simple builds.
2. **CGO-based library**: If no suitable pure-Go library exists or has critical limitations, use a CGO-based SMB library (e.g. `libsmbclient` bindings).
3. **Kernel mount (last resort)**: Only if both above approaches fail, fall back to mounting the SMB share via the OS and using the filesystem backend on the mount point. This must be clearly documented as platform-dependent and requiring elevated privileges.

**Configuration example:**

```yaml
storage_backend: smb
smb:
  server: "nas.local"
  share: "backups"
  username: "restic"
  password: "${SMB_PASSWORD}"
  domain: "WORKGROUP"
  port: 445
  base_path: "restic-repos"  # optional subdirectory within the share
```

**Acceptance criteria:**

- [x] New package `internal/storage/smb/` implementing `storage.Backend`
- [x] Connects to SMB share without OS-level mount (Go-native preferred)
- [x] Supports SMB2/SMB3 protocol
- [x] Supports authentication (username/password/domain)
- [x] Supports custom port configuration
- [x] Supports base path (subdirectory) within the share
- [x] Supports env var substitution for credentials (`${SMB_PASSWORD}`)
- [x] Atomic writes where possible (write to temp + rename)
- [x] Proper connection lifecycle management (connect/disconnect/reconnect)
- [x] Concurrent access safety (multiple goroutines reading/writing)
- [x] Integrated into config structs (`internal/config/`)
- [x] Integrated into backend factory in `cmd/serve.go`
- [x] Works with the instrumented storage wrapper for Prometheus metrics
- [x] Unit tests for backend logic
- [x] Integration test using a real SMB server (Docker-based, e.g. `dperson/samba` or similar)
- [x] Integration test exercises full restic lifecycle: init, backup, restore, verify, forget+prune
- [x] Integration test skips gracefully when Docker/SMB prerequisites are missing
- [x] Documentation in `docs/` describing setup and configuration
- [x] `config.example.yaml` updated with SMB section

---

### Task 1.2: NFS Storage Backend

**Priority:** MEDIUM
**Type:** Feature

**Description:**
Implement an NFS storage backend using a Go-native NFS client library (e.g. `github.com/vmware/go-nfs-client` or `github.com/willscott/go-nfs-client`). Similar to SMB, the server should connect to the NFS export directly over the NFS protocol without requiring a kernel mount. NFS is lower priority than SMB due to more complex permission management (uid/gid mapping, AUTH_SYS vs. Kerberos).

**Configuration example:**

```yaml
storage_backend: nfs
nfs:
  server: "nas.local"
  export: "/volume1/backups"
  base_path: "restic-repos"
  # uid/gid for NFS AUTH_SYS authentication
  uid: 1000
  gid: 1000
```

**Acceptance criteria:**

- [ ] New package `internal/storage/nfs/` implementing `storage.Backend`
- [ ] Connects to NFS export without OS-level mount (Go-native preferred)
- [ ] Supports NFSv3 or NFSv4 protocol
- [ ] Supports AUTH_SYS authentication (uid/gid)
- [ ] Supports base path within the export
- [ ] Proper connection lifecycle management
- [ ] Concurrent access safety
- [ ] Integrated into config structs and backend factory
- [ ] Works with the instrumented storage wrapper
- [ ] Unit tests for backend logic
- [ ] Integration test using a Docker-based NFS server
- [ ] Integration test exercises full restic lifecycle
- [ ] Integration test skips gracefully when prerequisites are missing
- [ ] Documentation and config example updated

---

## Epic 2: Web UI with Statistics Dashboard

### Task 2.1: Statistics Tracking Engine (SQLite)

**Priority:** HIGH
**Type:** Feature

**Description:**
Implement a statistics tracking layer that records per-repository traffic metrics (bytes written, bytes read, bytes deleted, request counts) during normal server operations. The tracking hooks into the storage backend (or middleware) and persists aggregated counters to a SQLite database. This avoids expensive filesystem traversals (like `du`) for size information and instead provides accurate traffic/usage data incrementally.

**Key design decisions:**

- Use SQLite as the persistence layer (lightweight, zero-config, embedded)
- Track metrics incrementally: on each read/write/delete operation, update counters
- Do NOT scan the filesystem or object store to compute repository sizes
- Ensure thread-safety for concurrent updates (SQLite WAL mode + proper locking or serialized writes)
- Provide an in-process Go API for querying stats (used by Web UI and potentially metrics)

**Tracked metrics per repository:**

- `bytes_written` (cumulative ingress)
- `bytes_read` (cumulative egress)
- `bytes_deleted` (cumulative)
- `write_count` (number of write operations)
- `read_count` (number of read operations)
- `delete_count` (number of delete operations)
- `last_access` (timestamp of last operation)
- `created_at` (timestamp of first operation)

**Configuration example:**

```yaml
stats:
  enabled: true
  db_path: "/data/stats.db"  # SQLite database path
```

**Acceptance criteria:**

- [ ] New package `internal/stats/` with SQLite-backed stats store
- [ ] Uses `modernc.org/sqlite` (pure Go) or `mattn/go-sqlite3` (CGO) for SQLite
- [ ] Schema auto-migration on startup
- [ ] Increment functions are safe for concurrent goroutine access
- [ ] Stats are updated on every storage operation (read, write, delete) per repository
- [ ] Provides query API: per-repo stats, all-repo summary, time-range queries
- [ ] Integrated as a storage wrapper (similar to `internal/storage/instrumented/`) or as middleware
- [ ] Configurable via config file (enable/disable, db path)
- [ ] Unit tests for stats store (concurrent writes, query correctness)
- [ ] Minimal performance overhead (batched writes or WAL mode)
- [ ] Graceful handling if stats DB is unavailable (log warning, don't crash server)

---

### Task 2.2: Web UI - Basic Framework & Repository Overview

**Priority:** HIGH
**Type:** Feature

**Description:**
Implement a server-side rendered web UI served at `/-/ui/` that displays repository information and traffic statistics. The UI uses Bootswatch (Bootstrap-based dark theme) with all assets embedded in the Go binary (no external CDN loads). Templates are rendered server-side using Go's `html/template`.

**Frontend asset management:**

- Use NPM locally to install Bootswatch (which includes Bootstrap)
- A build script (Makefile target or shell script) copies the required CSS/JS files from `node_modules/` into an `internal/ui/static/` directory
- Use Go's `embed` package to embed static assets and templates into the binary
- No runtime dependency on Node.js or NPM -- only needed at build time
- Bootswatch theme: use a dark theme (e.g. `darkly` or `slate`)

**UI pages:**

1. **Dashboard** (`/-/ui/`): Overview with total repository count, aggregate traffic stats
2. **Repository list** (`/-/ui/repos/`): Table of all repositories with per-repo stats (bytes in/out/deleted, last access)
3. **Repository detail** (`/-/ui/repos/{path}/`): Detailed stats for a single repository, lock management (see Task 2.3)

**Configuration example:**

```yaml
ui:
  enabled: true
  # Optional Basic Auth protection (plaintext credentials in config)
  auth:
    username: "admin"
    password: "secret"  # pragma: allowlist secret
```

**Acceptance criteria:**

- [ ] New package `internal/ui/` with handler, templates, and embedded static assets
- [ ] `package.json` at project root (or in `internal/ui/`) with Bootswatch as dependency
- [ ] Build script/Makefile target: `npm install` + copy CSS/JS to embed directory
- [ ] Bootswatch dark theme (e.g. `darkly`) used for all pages
- [ ] All static assets (CSS, JS, fonts) embedded via `//go:embed` -- nothing loaded from external URLs
- [ ] Server-side rendering with Go `html/template`
- [ ] Routes registered under `/-/ui/` prefix (similar to `/-/metrics`)
- [ ] Optional Basic Auth protection (username/password from config, plaintext)
- [ ] If auth not configured, UI is accessible without authentication
- [ ] Dashboard page showing: total repos, aggregate stats
- [ ] Repository list page showing: repo path, traffic stats, last access time
- [ ] Repository detail page showing: detailed per-repo stats
- [ ] Responsive layout (Bootstrap grid)
- [ ] No JavaScript required for core functionality (progressive enhancement OK)
- [ ] Works correctly when accessed through repo-prefix middleware (no path conflicts)
- [ ] Integration with stats engine (Task 2.1) for data
- [ ] `.gitignore` updated to exclude `node_modules/`

---

### Task 2.3: Web UI - Repository Lock Management

**Priority:** MEDIUM
**Type:** Feature

**Description:**
Add lock visibility and manual lock removal to the Web UI. Restic stores lock files as blobs of type `lock` in the repository. The server can list and delete these lock blobs without knowing the repository encryption password (lock files are encrypted, but the server manages them as opaque blobs). The UI should display lock metadata where available (lock blob names, sizes, timestamps) and allow manual deletion of individual locks.

**Important constraints:**

- The server NEVER needs or receives the repository password
- Lock blobs are opaque to the server (encrypted by restic client)
- The server can only list lock blob names/sizes and delete them
- The actual lock content (who locked, when, PID, etc.) is encrypted and not readable server-side
- The UI should clearly communicate that deleting a lock is a manual override and may cause issues if a backup is actively running

**Acceptance criteria:**

- [ ] Repository detail page (`/-/ui/repos/{path}/`) shows list of lock blobs (name, size, creation time if available from storage metadata)
- [ ] Each lock has a "Delete" button with a confirmation dialog
- [ ] Lock deletion calls the existing storage backend's `DeleteBlob` with type `lock`
- [ ] UI shows success/error feedback after lock deletion
- [ ] Warning text explaining that locks should only be removed if no backup is currently running
- [ ] Displays lock count on the repository list page
- [ ] Works with all storage backends (filesystem, memory, S3, WebDAV, SMB, NFS, etc.)
- [ ] CSRF protection on the delete action (e.g. token in form)

---

## Epic 3: GitHub Issues (Security & Reliability)

### Task 3.1: Graceful Shutdown Timeout and Tailscale Cleanup

**Priority:** HIGH
**Type:** Bug / Security
**GitHub Issue:** [#38](https://github.com/c-mueller/ts-restic-server/issues/38)
**Labels:** bug, security

**Description:**
Two shutdown-related issues: (1) `internal/server/server.go:73` uses `context.Background()` for `echo.Shutdown()`, causing potential indefinite hangs if request handlers block. (2) The Tailscale `tsnet.Server` is not always properly shut down during termination, leaving stale nodes or orphaned state.

**Acceptance criteria:**

- [ ] HTTP server shutdown uses a configurable timeout (default 30s) instead of unbounded `context.Background()`
- [ ] Configuration option: `shutdown_timeout: 30s` in config file
- [ ] `tsnet.Server.Close()` is called reliably in all shutdown paths (signal, error, context cancellation)
- [ ] `defer tsServer.Close()` verified to execute in all exit scenarios in `cmd/serve.go`
- [ ] Shutdown under load with active Tailscale connections completes cleanly
- [ ] Unit/integration test for shutdown timeout behavior

---

### Task 3.2: S3 Backend Request Body Size Limit

**Priority:** MEDIUM
**Type:** Enhancement / Security
**GitHub Issue:** [#39](https://github.com/c-mueller/ts-restic-server/issues/39)
**Labels:** enhancement, security

**Description:**
The S3 backend's `SaveConfig()` and `SaveBlob()` use `io.ReadAll(data)` which reads the entire request body into memory before uploading. Multiple concurrent uploads can compound memory usage. Short-term fix: add `io.LimitReader` guard. Long-term: investigate streaming upload via S3 PutObject with `Content-Length`.

**Acceptance criteria:**

- [ ] `io.LimitReader` wraps the reader in `SaveBlob()` and `SaveConfig()` with a configurable max size
- [ ] Server-wide `max_request_body_size` config option (uses Echo's `BodyLimit()` middleware)
- [ ] Sensible default limit (e.g. 256 MB or configurable)
- [ ] Error response with appropriate HTTP status (413) when limit exceeded
- [ ] Investigation documented: whether streaming upload to S3 is feasible with aws-sdk-go-v2
- [ ] If streaming is feasible: implement streaming PutObject (avoids full RAM buffering)
- [ ] Unit test verifying limit enforcement
- [ ] Existing integration tests still pass

---

### Task 3.3: Investigate rDNS Spoofing and Evaluate DoT/DoH Support

**Priority:** MEDIUM
**Type:** Investigation / Security
**GitHub Issue:** [#18](https://github.com/c-mueller/ts-restic-server/issues/18)
**Labels:** security, investigation

**Description:**
In plain mode, identity resolution via reverse DNS is inherently unauthenticated. An attacker controlling PTR records could spoof identity and match ACL rules. Cached results (600s TTL) amplify the impact window. Investigation needed for attack scenarios and potential DoT/DoH support.

**Acceptance criteria:**

- [ ] Attack scenario document written in `docs/` covering: same-LAN attacker, cross-network, compromised DNS, cache poisoning
- [ ] Each scenario documents: prerequisites, attack steps, impact, likelihood
- [ ] Feasibility assessment for DoT/DoH support completed (e.g. `github.com/miekg/dns`)
- [ ] Decision documented: implement DoT/DoH, recommend external resolver, or accept risk
- [ ] ACL documentation (`docs/acl.md`) updated with rDNS trust assumptions and threat model
- [ ] If DoT/DoH is implemented: config option, tests, documentation

---

### Task 3.4: Test ACL Engine in Plain Mode

**Priority:** MEDIUM
**Type:** Testing
**GitHub Issue:** [#8](https://github.com/c-mueller/ts-restic-server/issues/8)

**Description:**
The ACL engine has been manually tested in Tailscale mode but plain mode (rDNS-based identity) lacks end-to-end manual testing. Unit tests exist but do not cover the full integration path.

**Acceptance criteria:**

- [ ] rDNS identity resolution tested with system DNS
- [ ] rDNS identity resolution tested with custom `dns_server` config
- [ ] ACL rules matching on FQDN (e.g. `nas.home.arpa`) verified
- [ ] ACL rules matching on IP verified
- [ ] Trusted proxy support (`X-Forwarded-For` extraction with configured `trusted_proxies`) verified
- [ ] JSON error response on denial verified (should contain only `ip` field, no `hostname`/`user`/`tags`)
- [ ] Cache behavior verified (negative results, TTL expiry)
- [ ] Integration test or documented manual test procedure added

---

### Task 3.5: Metrics Cardinality Controls for Per-Host Labels

**Priority:** LOW
**Type:** Enhancement
**GitHub Issue:** [#40](https://github.com/c-mueller/ts-restic-server/issues/40)
**Labels:** enhancement

**Description:**
Per-identity and per-repo-path Prometheus label values create unbounded cardinality if exposed to broad networks. Currently bounded by Tailscale network size. Enhancement to add controls.

**Acceptance criteria:**

- [ ] Config option to make per-host metrics opt-in (`metrics.per_host_enabled: true/false`)
- [ ] When disabled, aggregate metrics are still collected (without identity/repo-path labels)
- [ ] Alternatively: cap tracked identities (top-N with "other" bucket) or hash-bucketed labels
- [ ] Chosen approach documented with rationale
- [ ] Existing metrics behavior unchanged when option is not configured (backward compatible)
- [ ] Unit test for cardinality-limited metrics

---

### Task 3.6: ACL Integration for Metrics Endpoint

**Priority:** LOW
**Type:** Enhancement
**GitHub Issue:** [#41](https://github.com/c-mueller/ts-restic-server/issues/41)
**Labels:** enhancement

**Description:**
The `/-/metrics` endpoint is registered outside the ACL middleware chain (intentional for monitoring access). Enhancement to optionally route it through ACL for unified access control.

**Acceptance criteria:**

- [ ] Config option: `metrics.acl_enabled: true/false` (default: false, preserving current behavior)
- [ ] When enabled, metrics endpoint passes through ACL middleware
- [ ] When disabled, current behavior preserved (separate Basic Auth)
- [ ] Documentation updated explaining both modes
- [ ] Test covering ACL-protected metrics endpoint

---

### Task 3.7: Configurable Verbosity for ACL Denial Responses

**Priority:** LOW
**Type:** Enhancement
**GitHub Issue:** [#42](https://github.com/c-mueller/ts-restic-server/issues/42)
**Labels:** enhancement

**Description:**
ACL denial responses currently include detailed identity information (hostname, user, tags, IP). This is useful for troubleshooting but may be undesirable in some deployments. Make it configurable.

**Acceptance criteria:**

- [ ] Config option: `acl.verbose_denials: true` (default: true, preserving current behavior)
- [ ] When `false`, denial response includes only: `{"status": 403, "error": "access denied", "request_id": "..."}`
- [ ] Detailed identity information still logged server-side regardless of setting
- [ ] Unit test for both verbose and minimal denial responses
- [ ] Documentation updated

---

## Priority Summary

| Priority | Task | Epic |
|----------|------|------|
| HIGH | Task 1.1: CIFS/SMB Storage Backend | Network Storage |
| HIGH | Task 2.1: Statistics Tracking Engine | Web UI |
| HIGH | Task 2.2: Web UI Framework & Repo Overview | Web UI |
| HIGH | Task 3.1: Graceful Shutdown (#38) | GitHub Issues |
| MEDIUM | Task 1.2: NFS Storage Backend | Network Storage |
| MEDIUM | Task 2.3: Lock Management UI | Web UI |
| MEDIUM | Task 3.2: S3 Body Size Limit (#39) | GitHub Issues |
| MEDIUM | Task 3.3: rDNS Spoofing Investigation (#18) | GitHub Issues |
| MEDIUM | Task 3.4: ACL Plain Mode Testing (#8) | GitHub Issues |
| LOW | Task 3.5: Metrics Cardinality Controls (#40) | GitHub Issues |
| LOW | Task 3.6: ACL for Metrics Endpoint (#41) | GitHub Issues |
| LOW | Task 3.7: ACL Denial Verbosity (#42) | GitHub Issues |

## Completed

- [x] Project initialization
- [x] Task 1.1: CIFS/SMB Storage Backend
- [x] Task 3.1: Graceful Shutdown Timeout (commit 477c861)
- [x] Task 2.1: Statistics Tracking Engine (SQLite)
- [x] Task 2.2: Web UI Framework & Repository Overview
- [x] Task 2.3: Web UI - Repository Lock Management
- [x] Task 3.2: S3 Backend Request Body Size Limit
- [x] Task 3.3: rDNS Spoofing Investigation
- [x] Task 3.7: ACL Denial Verbosity
- [x] Task 3.5: Metrics Cardinality Controls
- [x] Task 3.6: ACL for Metrics Endpoint
- [x] Task 3.4: ACL Plain Mode Testing

## Notes

- Epic 1 and Epic 2 (new features) have higher priority than Epic 3 (existing issues)
- CIFS/SMB backend is more important than NFS due to simpler permission model
- Web UI requires Stats Engine (Task 2.1) to be completed first
- All new backends must include Docker-based integration tests
- All UI assets must be embedded in the binary (no external CDN)
- Update this file after each major milestone
