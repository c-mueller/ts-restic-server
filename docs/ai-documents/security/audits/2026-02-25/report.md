# Security Audit Report — ts-restic-server

| Field          | Value                              |
|----------------|------------------------------------|
| Date           | 2026-02-25                         |
| Model          | Claude Opus 4.6                    |
| Scope          | Full source code review            |
| Commit         | 309a56e (master)                   |

---

## Executive Summary

This audit reviewed the complete source code of **ts-restic-server**, a Go-based Restic REST server with pluggable storage backends and optional Tailscale networking. The analysis covers API handlers, middleware, ACL engine, all storage backends (filesystem, memory, S3, WebDAV, rclone), configuration handling, server setup, Docker packaging, and dependencies.

Finding overview:

| Severity     | Count |
|--------------|-------|
| Critical     | 5     |
| High         | 8     |
| Medium       | 8     |
| Low          | 5     |
| Info         | 3     |
| **Total**    | **29**|

The most significant risks stem from the lack of built-in authentication, unencrypted default transport, missing input validation on blob names and repository paths, and unbounded request body reads that enable denial-of-service attacks.

---

## Critical Findings

### C-01: No Built-in Authentication Mechanism

- **Files:** `internal/api/router.go`, all API handlers
- **Severity:** CRITICAL

The server implements no authentication layer (no API keys, no Basic Auth, no bearer tokens). All access control relies exclusively on:

- Tailscale network isolation (optional, requires `listen_mode: tailscale`)
- ACL rules based on IP / hostname / Tailscale identity (optional, must be explicitly configured)

Without either measure enabled — which is the default — every request is accepted unconditionally.

Impact: Complete unauthorized access to all backup data and server operations.

Remediation:

- Implement at minimum API-key or Basic Auth support.
- Consider making ACL mandatory with a deny-by-default policy.
- Document clearly that the server must never be exposed to untrusted networks without explicit access control.

---

### C-02: Unencrypted Plain HTTP by Default

- **File:** `internal/server/listener.go:25-31`
- **Severity:** CRITICAL

In the default `listen_mode: plain`, the server listens on unencrypted TCP. All traffic — backup data, configuration, blob content — is transmitted in cleartext.

Impact: Man-in-the-middle attacks can intercept and modify data in transit. Complete loss of data confidentiality on untrusted networks.

Remediation:

- Add TLS support for plain mode (certificate file configuration).
- Consider making Tailscale mode (which provides TLS) the recommended default.
- Add prominent documentation warnings about plaintext exposure.

---

### C-03: Insecure Default Listen Address (0.0.0.0)

- **File:** `internal/config/config.go:74`
- **Severity:** CRITICAL

```go
viper.SetDefault("listen", ":8880")
```

The default binds to all network interfaces. Combined with C-01 (no authentication) this exposes the server to the entire network without any protection.

Impact: Remote attackers on the same network can access, modify, or delete all backup data.

Remediation:

- Change default to `127.0.0.1:8880`.
- Require explicit configuration for non-loopback binding.

---

### C-04: Missing Path Traversal Validation in Repo Prefix Middleware

- **File:** `internal/middleware/repoprefix.go:25-56`
- **Severity:** CRITICAL

The `RepoPrefix()` middleware extracts the repository path prefix from the URL but does not validate individual path segments for traversal sequences. A request like `GET /../../etc/passwd/config` would produce a prefix containing `../../etc/passwd`, which is passed directly to storage backends.

Impact: Directory traversal attacks — especially against the filesystem backend — could allow reading or writing files outside the intended repository directory.

Remediation:

```go
for _, seg := range segments {
    if seg == ".." || seg == "." || strings.ContainsAny(seg, "\x00\t\n\r") {
        return c.NoContent(http.StatusBadRequest)
    }
}
```

---

### C-05: Range Header Parsing Allows Negative Values

- **File:** `internal/api/blob.go:153-186`
- **Severity:** CRITICAL

The `parseRange()` function calls `strconv.ParseInt()` for the start and end values but never validates that the results are non-negative. A header like `Range: bytes=-100-5` would produce a negative `start` value passed to the storage backend.

Additionally, when the end value fails to parse (e.g., `Range: bytes=100-`), the function returns `(start, 0, true)`, which downstream logic silently expands to "from offset to end of file."

Impact: Out-of-bounds reads from the storage backend; potential information disclosure.

Remediation:

```go
if start < 0 || end < 0 || start > end {
    return 0, 0, false
}
```

---

## High Findings

### H-01: No Blob Name Sanitization

- **File:** `internal/api/blob.go:149-151`
- **Severity:** HIGH

```go
func blobParams(c echo.Context) (storage.BlobType, string) {
    return storage.BlobType(c.Param("type")), c.Param("name")
}
```

Blob names are extracted directly from the URL with no validation. Names containing `../`, `\`, or null bytes are passed unchanged to storage backends. This compounds the path traversal risk from C-04.

Impact: Path traversal via crafted blob names; unauthorized file access in the filesystem backend.

Remediation:

- Reject names containing `/`, `\`, `..`, or control characters.
- Consider validating against the expected format (hex strings for restic blobs).

---

### H-02: Unbounded Request Body Reads (Memory Exhaustion DoS)

Files:

- `internal/storage/memory/memory.go:179` (`io.ReadAll` in SaveBlob)
- `internal/storage/s3/s3.go:136,193` (`io.ReadAll` in SaveBlob/SaveConfig)
- `internal/api/config_handler.go:46-62` (SaveConfig handler)
- `internal/api/blob.go:100-121` (SaveBlob handler)

Severity: HIGH

No handler applies `http.MaxBytesReader` or any other size limit before reading request bodies. The memory backend's `io.ReadAll()` call is particularly dangerous — it buffers the entire upload in RAM **before** checking the quota.

Impact: A single request with an arbitrarily large body can exhaust server memory and crash the process.

Remediation:

- Wrap request bodies with `http.MaxBytesReader` at the handler or middleware level.
- In the memory backend, use `io.LimitReader(data, maxBytes+1)` before `io.ReadAll`.

---

### H-03: Credentials Stored in Plaintext Configuration

Files:

- `internal/config/config.go:64-71` (S3, WebDAV, Rclone credential structs)
- `config.example.yaml:18-31`

Severity: HIGH

S3 access keys/secret keys, WebDAV username/password, and Rclone credentials are stored as plain strings in configuration structs and YAML files. There is no redaction in logs or error messages, no memory zeroization, and no integration with secrets management systems.

Impact: Credential leakage through config files, process environment, logs, or memory dumps.

Remediation:

- Support reading credentials from external secret stores or files with restricted permissions.
- Implement a `SensitiveString` type that suppresses marshaling/logging.
- Document that config files containing credentials must have `0600` permissions.

---

### H-04: ACL Middleware Empty Identity Bypass

- **File:** `internal/middleware/acl.go:19-28`
- **Severity:** HIGH

```go
identities := GetIdentity(c.Request().Context())
if identities == nil {
    identities = []string{c.RealIP()}
}
```

The check uses `identities == nil`, which does **not** catch an empty slice `[]string{}`. If the identity middleware succeeds but returns an empty slice, the ACL engine receives empty identities — potentially leading to unexpected allow/deny decisions.

Impact: ACL bypass or incorrect access decisions when identity resolution returns empty results.

Remediation:

```go
if identities == nil || len(identities) == 0 {
    identities = []string{c.RealIP()}
}
```

---

### H-05: Identity Spoofing via rDNS in Plain Mode

- **File:** `internal/middleware/identity.go:88-143`
- **Severity:** HIGH

In plain mode, identities are resolved via reverse DNS lookups. DNS is unauthenticated — an attacker controlling their PTR records can impersonate any hostname. Results are cached for 600 seconds (default), amplifying the window of exploitation.

Impact: ACL bypass by forging rDNS-based identities.

Remediation:

- Document that rDNS-based identity is inherently weak and recommend Tailscale for trusted environments.
- Prefer IP-based ACL rules when using plain mode.
- Add forward-confirmed reverse DNS (FCrDNS) validation.

---

### H-06: Missing HTTP Security Headers

- **File:** `internal/api/router.go`
- **Severity:** HIGH

No security headers are set on responses:

- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `Cache-Control: no-store` (to prevent caching of blob data)
- `Strict-Transport-Security` (when TLS is active)

Impact: Increased attack surface for MIME-sniffing, clickjacking, and sensitive data caching by intermediaries.

Remediation: Add a middleware that sets these headers on every response.

---

### H-07: Backend Credentials Sent Over Unverified HTTP

Files:

- `internal/storage/rclone/rclone.go:276-279` (Basic Auth)
- `internal/storage/webdav/webdav.go:23-29` (Basic Auth)

Severity: HIGH

Both the Rclone and WebDAV backends send credentials using HTTP Basic Auth. While config validation checks the endpoint scheme, there is no runtime enforcement — if the URL is HTTP, credentials are sent in cleartext.

Impact: Credential interception via network sniffing.

Remediation: Reject non-HTTPS endpoints at runtime when credentials are configured.

---

### H-08: Blob Type Case Sensitivity

- **File:** `internal/api/blob.go:150`, `internal/storage/types.go`
- **Severity:** HIGH

Blob type validation uses a case-sensitive map lookup (`storage.ValidBlobTypes[t]`). The type parameter is cast directly from the URL without normalization. If Echo's router or a reverse proxy normalizes URL casing, a request for `/Data/blobname` could bypass type validation.

Impact: Potential type-confusion attacks; inconsistent validation between layers.

Remediation:

```go
t := storage.BlobType(strings.ToLower(c.Param("type")))
```

---

## Medium Findings

### M-01: Filesystem Backend Follows Symlinks

- **File:** `internal/storage/filesystem/filesystem.go`
- **Severity:** MEDIUM

The filesystem backend uses standard Go file operations (`os.Open`, `filepath.Walk`, `os.Rename`) that follow symbolic links. If an attacker can place symlinks inside the repository directory, they could read or write files outside the intended storage tree.

Impact: File-system escape if symlinks exist in the data directory.

Remediation:

- Resolve paths with `filepath.EvalSymlinks()` and verify they remain under the base path.
- Use `os.Lstat` to detect symlinks before following.

---

### M-02: Filesystem Atomic Write TOCTOU Race Condition

- **File:** `internal/storage/filesystem/filesystem.go:193-227`
- **Severity:** MEDIUM

The `atomicWrite` function creates a temporary file then renames it to the final path. Between these operations, a concurrent process could interfere with the target path. While `os.Rename` itself is atomic, the overall sequence is not fully safe under concurrent access.

Impact: Rare data corruption or lost writes under high concurrency.

Remediation: Use `O_EXCL` flags or handle already-exists cases explicitly.

---

### M-03: Identity Cache Not Bounded

- **File:** `internal/middleware/identity.go:44-79, 164-184`
- **Severity:** MEDIUM

The rDNS and WhoIs identity caches use unbounded `map[string]` structures. An attacker sending requests from many distinct IPs can grow the cache indefinitely.

Impact: Gradual memory exhaustion; DoS via cache growth.

Remediation: Use a bounded LRU cache with a configurable maximum size.

---

### M-04: ACL Wildcard Identity Overly Permissive

- **File:** `internal/acl/acl.go:195-208`
- **Severity:** MEDIUM

The `*` wildcard identity matches any requester. Combined with cascading path rules (deepest path wins), a wildcard rule at a deeper path can inadvertently override a deny rule at a shallower path.

Impact: Misconfiguration can grant broader access than intended.

Remediation:

- Add configuration validation that warns about wildcard rules overriding deny rules.
- Document cascading behavior and wildcard semantics prominently.

---

### M-05: Append-Only Mode Incomplete

- **File:** `internal/api/blob.go:123-135`
- **Severity:** MEDIUM

Append-only mode blocks blob deletion but does not prevent:

- Config overwrite (which could disable append-only).
- Repository recreation via DELETE + POST `/?create=true`.

Impact: Append-only guarantees can be circumvented through config or repo-level operations.

Remediation: Extend append-only enforcement to config writes and repo deletion.

---

### M-06: S3 Error Detection Uses String Matching

- **File:** `internal/storage/s3/s3.go:291-303`
- **Severity:** MEDIUM

```go
return strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "NoSuchKey")
```

The `isNotFound` function falls back to matching error message strings, which is fragile and may misclassify unrelated errors.

Impact: Incorrect HTTP status codes; silent data loss if a network error is treated as "not found."

Remediation: Remove the string fallback and rely solely on typed error assertions. Log unmatched errors for investigation.

---

### M-07: Query Strings Logged Without Redaction

- **File:** `internal/middleware/logger.go:20-21`
- **Severity:** MEDIUM

The request logger records the full raw query string. While current API endpoints use only `?create=true`, future extensions or misconfigured clients could include sensitive parameters.

Impact: Potential credential or token leakage through log files.

Remediation: Add a redaction mechanism for known-sensitive query parameters (e.g., `token`, `password`).

---

### M-08: No Rate Limiting

- **Files:** All API endpoints
- **Severity:** MEDIUM

The server has no rate limiting. Attackers can:

- Flood identity resolution with rDNS lookups from many source IPs.
- Exhaust connections and backend I/O.
- Brute-force repository paths.

Impact: Denial of service; resource exhaustion.

Remediation: Add per-IP rate limiting middleware.

---

## Low Findings

### L-01: Accept Header Comparison Case-Sensitive

- **File:** `internal/api/version.go:10-13`
- **Severity:** LOW

API version negotiation uses strict string comparison on the Accept header. Per HTTP spec, media types should be compared case-insensitively.

Remediation: Normalize to lowercase before comparison.

---

### L-02: Inconsistent HTTP Status Codes

- **Files:** `internal/api/repo.go`, `internal/api/blob.go`, `internal/api/config_handler.go`
- **Severity:** LOW

Different error conditions return different status codes (400, 403, 404, 507) without a consistent JSON error envelope, making it difficult for clients to programmatically handle errors.

Remediation: Standardize error responses with a JSON body containing error code and message.

---

### L-03: Tailscale State Directory Permissions Not Verified

- **File:** `cmd/serve.go:70`
- **Severity:** LOW

The state directory is created with `0700`, but if the directory already exists, its permissions are not verified.

Remediation: Check and warn if existing directory permissions are too open.

---

### L-04: Docker Image Not Pinned to Digest

- **File:** `Dockerfile:1,8`
- **Severity:** LOW

Base images (`golang:1.26-bookworm`, `debian:bookworm-slim`) are referenced by tag, not by digest. Tags are mutable and could be replaced with compromised images.

Remediation: Pin base images to specific SHA256 digests.

---

### L-05: Panic Recovery May Fail After Partial Response

- **File:** `internal/middleware/recover.go:10-26`
- **Severity:** LOW

If a panic occurs after the handler has already written response headers, the recovery middleware's `c.NoContent(500)` call will fail silently, leaving the client with a partial/corrupt response.

Remediation: Check `c.Response().Committed` before attempting to write the error response.

---

## Informational Findings

### I-01: ListBlobs Returns Basename Only (Sharded Filesystem)

- **File:** `internal/storage/filesystem/filesystem.go:146`

When data sharding is enabled, `ListBlobs` returns only `info.Name()` (the file basename). Restic expects the bare blob name without shard prefixes, so this is likely correct — but should be verified against the restic REST API specification.

---

### I-02: No Symlink Consideration in ACL Path Matching

- **File:** `internal/acl/acl.go:178-192`

ACL rules use string-based path matching. If the filesystem backend resolves symlinks to different actual paths, ACL rules based on the logical path may not apply to the resolved path.

---

### I-03: Recover Middleware Logs Full Panic Values

- **File:** `internal/middleware/recover.go:18`

`zap.Any("panic", r)` logs the complete panic value, which may include stack traces or internal state. While helpful for debugging, this could leak implementation details if logs are accessible to attackers.

---

## Prioritized Remediation Roadmap

### Immediate (before any network exposure)

1. **Add authentication** (C-01) — API key or Basic Auth at minimum.
1. **Add TLS support for plain mode** (C-02) — or enforce Tailscale-only deployment.
1. **Change default listen address** to `127.0.0.1:8880` (C-03).
1. **Validate repo prefix paths** for traversal sequences (C-04).
1. **Sanitize blob names** — reject `..`, `/`, `\`, null bytes (H-01).

### Short-term (before production use)

1. **Limit request body size** via `http.MaxBytesReader` (H-02).
1. **Fix range parsing** to reject negative values (C-05).
1. **Fix ACL empty-identity check** (H-04).
1. **Add security response headers** (H-06).
1. **Enforce HTTPS for credentialed backends** (H-07).

### Medium-term (production hardening)

1. **Bound identity caches** with LRU eviction (M-03).
1. **Extend append-only enforcement** to config and repo operations (M-05).
1. **Add rate limiting** (M-08).
1. **Implement credential redaction** in logs and error messages (H-03, M-07).
1. **Add FCrDNS validation** for rDNS identity mode (H-05).
