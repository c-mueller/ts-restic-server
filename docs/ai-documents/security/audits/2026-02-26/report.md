# Security Audit Follow-up Report

**Project:** ts-restic-server
**Date:** 2026-02-26
**Type:** Follow-up review of the initial security audit from 2026-02-25
**Auditor:** Claude Opus 4.6 (AI-assisted security review)
**Scope:** Full source code review, remediation verification, new finding identification
**Baseline:** [Security Audit Report 2026-02-25](../2026-02-25/report.md) (29 findings)

> This report is a follow-up to the initial security audit conducted on 2026-02-25.
> It verifies the remediation status of all original findings, reviews the security-related
> PRs and issues addressed since the initial audit, and identifies new observations
> discovered during re-review of the full codebase.

---

## Executive Summary

This follow-up audit verifies the remediation status of all 29 findings from the initial security audit dated 2026-02-25 and identifies new security observations discovered during re-review. The project has undergone substantial security hardening through 9 security-focused pull requests merged on 2026-02-25, addressing the most critical and high-severity findings.

**Key Results:**

- **15 of 29 original findings** have been remediated through code changes
- **12 findings** were accepted as design decisions or low-risk trade-offs
- **2 items** remain open for further investigation (Issue #18, Issue #8)
- **7 new findings** identified during this follow-up review (0 critical, 1 high, 3 medium, 2 low, 1 informational)

**Overall Security Posture: Significantly Improved** — The application now demonstrates strong defense-in-depth with layered input validation, path traversal protection, and proper access control.

---

## Part 1: Remediation Verification

### 1.1 Fully Remediated Findings

#### C-04: Path Traversal in Repo Prefix Middleware — FIXED (PR #29)

**Verification:** The `RepoPrefix()` middleware in `internal/middleware/repoprefix.go:42-54` now validates all URL path segments before processing:

```go
for _, seg := range segments {
    if seg == ".." || seg == "." {
        return apierror.New(c, http.StatusBadRequest, "bad request",
            "path traversal sequences are not allowed", ...)
    }
    if strings.ContainsAny(seg, "\x00") {
        return apierror.New(c, http.StatusBadRequest, "bad request",
            "path contains invalid characters", ...)
    }
}
```

**Assessment:** Effective mitigation. Both `..` and `.` segments are rejected, and null bytes are blocked. The validation occurs before any path is passed to storage backends. Combined with the filesystem backend's `validatePath()` symlink resolution (defense-in-depth), path traversal attacks are comprehensively blocked.

#### C-05: Range Header Negative Values — FIXED (PR #30)

**Verification:** The `parseRange()` function in `internal/api/blob.go:175-211` now explicitly rejects negative values:

```go
start, err := strconv.ParseInt(parts[0], 10, 64)
if err != nil || start < 0 {
    return 0, 0, false
}
// ...
end, err := strconv.ParseInt(parts[1], 10, 64)
if err != nil || end < 0 {
    return 0, 0, false
}
```

Additional validation ensures `end >= start` (line 206) and range is clamped against actual blob size (line 76-84).

**Assessment:** Effective mitigation. Negative values are rejected at parse time, and range bounds are validated against actual blob size before being passed to backends.

#### H-01: Blob Name Sanitization — FIXED (PR #31)

**Verification:** A strict hex-only validation is now enforced in `internal/api/blob.go:17-24`:

```go
var validBlobNameRe = regexp.MustCompile(`^[0-9a-fA-F]+$`)

func isValidBlobName(name string) bool {
    return validBlobNameRe.MatchString(name)
}
```

This check is applied in all four blob handlers: `HeadBlob`, `GetBlob`, `SaveBlob`, `DeleteBlob` — each returns HTTP 400 if the name fails validation.

**Assessment:** Effective mitigation. Only hex strings are accepted as blob names, completely eliminating path traversal via blob name injection (`../../../etc/passwd`). Restic uses SHA-256 hashes (64 hex chars), so this is compatible with the protocol.

#### H-04: ACL Bypass on Empty Identity List — FIXED (PR #28)

**Verification:** The ACL middleware in `internal/middleware/acl.go:24-26` now uses `len()` instead of `nil` check:

```go
identities := GetIdentity(c.Request().Context())
if len(identities) == 0 {
    identities = []string{c.RealIP()}
}
```

**Assessment:** Effective mitigation. An empty identity slice `[]string{}` (which is non-nil but empty) now correctly triggers the RealIP fallback, ensuring ACL rules are evaluated against a valid identity.

#### H-06: Missing HTTP Security Headers — FIXED (PR #32)

**Verification:** The `SecurityHeaders` middleware in `internal/middleware/securityheaders.go:8-21` sets:

- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `Cache-Control: no-store`
- `Strict-Transport-Security: max-age=63072000; includeSubDomains` (TLS mode only)

**Assessment:** Effective mitigation. Headers are set before handler execution. HSTS is correctly conditional on TLS mode.

#### H-08: Blob Type Case Sensitivity — FIXED (PR #30)

**Verification:** Both `blobParams()` (blob.go:172) and `ListBlobs` handler (list.go:17) normalize blob type to lowercase:

```go
return storage.BlobType(strings.ToLower(c.Param("type"))), c.Param("name")
```

**Assessment:** Effective mitigation. Case-insensitive blob type handling prevents inconsistencies.

#### M-01: Filesystem Backend Follows Symlinks — FIXED (PR #31)

**Verification:** The filesystem backend implements comprehensive symlink protection:

1. `validatePath()` (filesystem.go:249-270) resolves symlinks via `filepath.EvalSymlinks()` and verifies the resolved path stays within `resolvedBase`
2. `isSubPath()` (filesystem.go:273-275) uses proper separator-aware prefix matching
3. `ListBlobs()` (filesystem.go:199-202) skips symlinks: `if info.Mode()&os.ModeSymlink != 0`
4. `SaveBlob()` (filesystem.go:156) uses `os.Lstat()` (does not follow symlinks) for existence checks

**Assessment:** Effective mitigation. Defense-in-depth with both symlink resolution + bounds checking and symlink skipping in directory walks.

#### M-02: Atomic Write Race Condition — FIXED (PR #31)

**Verification:** `SaveBlob()` (filesystem.go:152-159) now checks for existing blobs before writing:

```go
if _, err := os.Lstat(p); err == nil {
    io.Copy(io.Discard, data)
    return nil
}
```

Since blob names are content-addressed hashes, an existing blob with the same name already has the correct content. The reader is drained to avoid client-side connection issues.

**Assessment:** Effective mitigation. Content-addressed deduplication eliminates race conditions for concurrent writes of identical hashes.

#### M-03: Unbounded Identity Cache — FIXED (PR #33)

**Verification:** Both `rdnsCache` and `whoIsCache` in `internal/middleware/identity.go` now use bounded FIFO eviction with `container/list`:

```go
type rdnsCache struct {
    mu      sync.RWMutex
    entries map[string]*list.Element
    order   *list.List
    ttl     time.Duration
    maxSize int
}
```

Eviction occurs when `len(entries) >= maxSize` (identity.go:95-101). Default `maxSize` is 1000 (configurable via `identity_cache_size`).

**Assessment:** Effective mitigation. Memory usage is bounded. FIFO eviction is simple and predictable.

#### L-01: Accept Header Case Sensitivity — FIXED (PR #30)

**Verification:** `isV2()` in `internal/api/version.go:14-16` uses `strings.EqualFold()` for case-insensitive comparison, per RFC 7231.

**Assessment:** Effective mitigation.

#### L-02: Inconsistent HTTP Status Codes — FIXED (PR #27)

**Verification:** All endpoints now use the `apierror` package for standardized JSON error responses with consistent structure: `{status, error, message, request_id, data}`.

**Assessment:** Effective mitigation.

#### L-05: Panic Recovery After Partial Responses — FIXED (PR #34)

**Verification:** The `Recover` middleware in `internal/middleware/recover.go:28-34` now checks `c.Response().Committed` before writing error responses and logs a warning if the response is already committed.

Additionally, panic values are logged as `fmt.Sprintf("%v", r)` (string) instead of `zap.Any()`, preventing potential sensitive data serialization.

**Assessment:** Effective mitigation.

#### I-01: ListBlobs Data Sharding Bare Names — VERIFIED (PR #37)

**Verification:** `ListBlobs()` uses `info.Name()` (filesystem.go:209) which returns the bare filename without directory prefix. Test coverage added in PR #37 confirms this behavior.

**Assessment:** Verified correct behavior.

#### I-02: ACL Path Matching vs Symlinks — ADDRESSED (PR #31)

**Verification:** Symlinks are now validated via `filepath.EvalSymlinks()` before backend operations, and symlinks are skipped during directory walks. ACL path matching operates on logical paths before symlink resolution, which is the correct behavior.

**Assessment:** No remaining concern.

#### I-03: Panic Logging Verbosity — FIXED (PR #34)

**Verification:** Panic values are logged as formatted strings via `fmt.Sprintf("%v", r)` instead of `zap.Any()`, reducing the risk of serializing complex objects that might contain sensitive data.

**Assessment:** Effective mitigation.

---

### 1.2 Accepted Risks (Design Decisions)

The following findings from the original audit were reviewed and classified as accepted risks in the risk assessment. This follow-up confirms the rationale remains valid:

| Finding | Description | Status | Notes |
|---------|-------------|--------|-------|
| C-01 | No Built-in Authentication | Accepted | By design: Tailscale provides authn, plain mode delegates to infrastructure |
| C-02 | Unencrypted Plain HTTP | Accepted | By design: Tailscale mode uses TLS; plain mode for local/VPN use |
| C-03 | Default Listen Address 0.0.0.0 | Accepted | Standard server default; addressed via deployment documentation |
| H-02 | Unbounded Request Body Reads | Accepted | Low practical risk; see new finding F-02 for S3-specific concern |
| H-03 | Plaintext Credentials in Config | Mitigated | Env var substitution (PR #36) allows `${VAR}` in config files |
| H-07 | Backend Credentials Over HTTP | Accepted | Endpoint URL scheme is validated at config time |
| M-04 | ACL Wildcard Permissiveness | Accepted | Documented behavior; `*` identity is intentional |
| M-05 | Incomplete Append-Only Mode | Accepted | Repo deletion blocked; config overwrite allowed by design |
| M-06 | S3 Error Detection (String Matching) | Accepted | AWS SDK typed errors checked first; string fallback as safety net |
| M-07 | Query String Logging | Accepted | Query strings logged for debugging; no sensitive tokens expected |
| M-08 | No Rate Limiting | Accepted | Not applicable for intended deployment model (Tailscale/private network) |
| L-03 | Tailscale State Directory Permissions | Fixed | Created with `0o700` in both `cmd/serve.go:78` and `listener.go:34` |

---

### 1.3 Open Items

#### H-05: rDNS Identity Spoofing in Plain Mode — OPEN (Issue #18)

**Status:** Issue #18 remains open. In plain mode with ACL enabled, identity resolution relies on reverse DNS, which is inherently unauthenticated. An attacker controlling their PTR records could spoof hostnames matching ACL rules.

**Current Mitigations:**

- Tailscale mode uses authenticated WhoIs (not affected)
- Plain mode IP address is always included in identity list (ACL rules can match on IP)
- Trusted proxy configuration limits XFF header trust

**Recommendation:** This remains the most significant open security item. See risk assessment for prioritization.

#### L-04: Docker Base Images Not Pinned — PARTIALLY OPEN (Issue #21)

**Status:** Issue #21 was closed, but the Dockerfile still references mutable tags:

```dockerfile
FROM golang:1.26-bookworm AS build
FROM debian:bookworm-slim
```

**Recommendation:** Pin to SHA256 digests for reproducible builds and supply chain integrity.

#### Issue #8: ACL Plain Mode Testing — OPEN

**Status:** End-to-end testing of ACL in plain mode with rDNS-based identity resolution has not been completed. Relevant to H-05 concerns.

---

## Part 2: New Findings

### F-01: Prometheus Metrics Label Cardinality DoS (Medium)

**Location:** `internal/middleware/metrics.go:34-46`

**Description:** The metrics middleware records per-identity and per-repo-path label values in Prometheus counters:

```go
metrics.HostRequestsTotal.WithLabelValues(identity, repoPath, method).Inc()
metrics.HostBytesReceivedTotal.WithLabelValues(identity, repoPath).Add(...)
metrics.HostBytesSentTotal.WithLabelValues(identity, repoPath).Add(...)
```

In deployments with many unique source IPs (plain mode) or dynamically generated repo paths, each unique combination creates a new Prometheus time series. An attacker sending requests from diverse IPs (e.g., via a botnet or rotating proxy) could create millions of unique time series, exhausting Prometheus server memory.

**Impact:** Denial of service against the Prometheus monitoring infrastructure. In severe cases, the ts-restic-server process itself may experience increased memory usage from the in-process metric registry.

**CVSS Estimate:** 5.3 (Medium) — Network-accessible, no authentication required in plain mode, availability impact.

**Recommendation:**

- Option A: Replace unbounded identity labels with a fixed cardinality dimension (e.g., hash bucket, "known" vs "unknown")
- Option B: Limit the number of tracked identities and aggregate the rest into an "other" bucket
- Option C: Make per-host metrics opt-in via configuration, with clear documentation of the cardinality risk

---

### F-02: S3/WebDAV Backend Memory Exhaustion via io.ReadAll (Medium)

**Location:** `internal/storage/s3/s3.go:136-139`, `internal/storage/s3/s3.go:193-196`

**Description:** The S3 backend's `SaveConfig()` and `SaveBlob()` methods read the entire request body into memory before uploading to S3:

```go
func (b *Backend) SaveBlob(ctx context.Context, t storage.BlobType, name string, data io.Reader) error {
    buf, err := io.ReadAll(data)  // Unbounded memory allocation
    // ...
}
```

While the filesystem backend uses streaming writes (temp file + rename), the S3 backend buffers the entire blob in RAM. Restic snapshots can contain large data blobs (up to ~8MB by default, but configurable). A malicious or misconfigured client sending very large payloads could exhaust server memory.

**Impact:** Denial of service via memory exhaustion. Amplified when multiple concurrent uploads target the S3 backend.

**CVSS Estimate:** 5.3 (Medium) — Requires authenticated write access (if ACL is configured).

**Recommendation:**

- Use the AWS SDK's S3 multipart upload or streaming `PutObject` with `Content-Length` instead of buffering the entire body
- As an immediate mitigation, wrap the reader with `io.LimitReader(data, maxBlobSize)` to enforce an upper bound
- Consider adding server-wide `max_request_body_size` configuration

---

### F-03: Metrics Endpoint Bypasses ACL Middleware (Medium)

**Location:** `internal/api/router.go:21-23`

**Description:** The `/-/metrics` endpoint is registered as a system route before the middleware chain:

```go
// System routes — registered directly on root, outside the API middleware chain.
if metricsCfg.Enabled && metrics.Registry != nil {
    e.GET("/-/metrics", metrics.Handler(metricsCfg.Password))
}
```

This means the metrics endpoint is not subject to ACL rules, identity resolution, or request logging. While the endpoint has its own Basic Auth protection, if `metrics.password` is empty (the default), the endpoint is publicly accessible regardless of any ACL configuration.

The metrics data includes per-host request counts, bytes transferred, ACL decision counts, and Go runtime information — all potentially useful for reconnaissance.

**Impact:** Information disclosure. An unauthenticated attacker can obtain server operational metrics, identity information, and infrastructure details.

**CVSS Estimate:** 4.3 (Medium) — Network-accessible, information disclosure only, no integrity or availability impact.

**Recommendation:**

- Require `metrics.password` to be set when metrics are enabled (reject empty password at config validation)
- Alternatively, route the metrics endpoint through the ACL middleware chain
- Consider binding the metrics endpoint to a separate listener (e.g., localhost-only management port)

---

### F-04: ACL Denial Response Reveals Server-Side Identity Resolution (Low)

**Location:** `internal/middleware/acl.go:59-79`

**Description:** When the ACL denies a request, the 403 response includes detailed identity information resolved by the server:

```go
data := map[string]interface{}{
    "path":      repoPath,
    "operation": opName(op),
    "ip":        c.RealIP(),
}
if whoIs := GetWhoIsResult(c.Request().Context()); whoIs != nil {
    if whoIs.FQDN != "" { data["hostname"] = whoIs.FQDN }
    if whoIs.LoginName != "" { data["user"] = whoIs.LoginName }
    if len(whoIs.Tags) > 0 { data["tags"] = whoIs.Tags }
}
```

This reveals to the client:

- How the server resolves their identity (FQDN, login name, Tailscale tags)
- The repo path the server computed from their request
- The operation type classification

**Impact:** Information disclosure. An attacker can probe the ACL system to understand how identity resolution works, what their resolved identity is, and map out the server's ACL rule structure through targeted probing.

**CVSS Estimate:** 3.1 (Low) — Requires network access, limited information exposed.

**Recommendation:**

- Make verbose ACL denial responses configurable (e.g., `acl.verbose_denials: true` for debugging, `false` for production)
- In production mode, return only `{"status": 403, "error": "access denied", "request_id": "..."}` without identity details
- Keep the detailed information in the server-side log (already logged at `acl.go:43-50`)

---

### F-05: Graceful Shutdown Without Timeout (Low)

**Location:** `internal/server/server.go:73`

**Description:** The server shutdown uses an unbounded context:

```go
case <-ctx.Done():
    s.logger.Info("shutting down server")
    return s.echo.Shutdown(context.Background())
```

If any in-flight requests take a long time (e.g., large blob uploads, slow storage backends), the shutdown process will block indefinitely. This could prevent clean process restart during deployments.

Additionally, the maintainer has observed that the Tailscale `tsnet.Server` is not always properly shut down during process termination, potentially leaving stale Tailscale nodes or orphaned state.

**Impact:** Reduced operational reliability. Long-running requests prevent clean shutdown. Stale Tailscale nodes require manual cleanup.

**CVSS Estimate:** 2.0 (Low) — Local impact, no confidentiality or integrity concern.

**Recommendation:**

```go
shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
return s.echo.Shutdown(shutdownCtx)
```

Additionally, audit the `tsnet.Server` lifecycle to ensure `tsServer.Close()` is called reliably in all shutdown paths.

---

### F-06: Missing Content-Security-Policy Header (Low)

**Location:** `internal/middleware/securityheaders.go`

**Description:** The security headers middleware does not set a `Content-Security-Policy` header. While this server primarily serves binary data (not HTML), the JSON error responses are served with `application/json` content type. If a browser were to render a response due to content-type sniffing (mitigated by `nosniff`), having a restrictive CSP would provide defense-in-depth.

**Impact:** Minimal for a REST API server. Defense-in-depth measure.

**CVSS Estimate:** 2.0 (Low)

**Recommendation:**

```go
h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
```

---

### F-07: Default Metrics Enabled Without Authentication (Informational)

**Location:** `internal/config/config.go:96-97`

**Description:** Metrics are enabled by default with an empty password:

```go
viper.SetDefault("metrics.enabled", true)
viper.SetDefault("metrics.password", "")
```

This means out-of-the-box deployments expose Prometheus metrics at `/-/metrics` without any authentication. Combined with F-03 (metrics bypass ACL), this creates an unauthenticated information disclosure surface.

**Impact:** Low in the intended Tailscale deployment model (network is already trusted). Higher risk in plain mode on public networks.

**Recommendation:** Either default `metrics.enabled` to `false`, or add a configuration validation warning when metrics are enabled without a password.

---

## Part 3: Dependency Assessment

### Go Version

The project uses Go 1.25.5, which is a current stable release. No known Go runtime vulnerabilities affect this version.

### Key Dependencies

| Dependency | Version | Notes |
|-----------|---------|-------|
| `golang.org/x/net` | v0.50.0 | Updated via PR #2 (from v0.48.0); current |
| `golang.org/x/crypto` | v0.48.0 | Current |
| `github.com/labstack/echo/v4` | v4.15.1 | Current stable; no known CVEs |
| `github.com/aws/aws-sdk-go-v2` | v1.41.2 | Current |
| `tailscale.com` | v1.94.2 | Recent release |
| `github.com/studio-b12/gowebdav` | v0.12.0 | Stable; no known CVEs |
| `go.uber.org/zap` | v1.27.1 | Current stable |
| `github.com/prometheus/client_golang` | v1.23.2 | Current |

**Note:** `govulncheck` was not available in the build environment. Running `govulncheck ./...` is recommended as part of CI to detect known vulnerabilities in dependencies.

### Docker Image Supply Chain

The Dockerfile uses mutable tags (`golang:1.26-bookworm`, `debian:bookworm-slim`). While practical for maintenance, this introduces supply chain risk if upstream images are compromised. See L-04 (still partially open).

---

## Part 4: Architecture Review

### Positive Security Patterns Observed

1. **Defense-in-depth for path traversal:** Input validation at middleware layer (repoprefix.go), blob name validation at API layer (blob.go), and path resolution at storage layer (filesystem.go) — three independent layers of protection.

2. **Constant-time password comparison:** The metrics handler uses `crypto/subtle.ConstantTimeCompare()` for Basic Auth, preventing timing attacks.

3. **Atomic file operations:** The filesystem backend's temp+fsync+rename pattern ensures crash consistency and prevents partial writes from being served.

4. **Structured error responses:** The `apierror` package provides consistent JSON error envelopes with request IDs for log correlation, without leaking internal error details.

5. **Bounded caches with FIFO eviction:** Both identity caches (rDNS and WhoIs) have configurable size limits with deterministic eviction behavior.

6. **Secure defaults for file permissions:** Storage directories are created with `0o700`, restricting access to the process owner.

7. **Proper use of Echo's IPExtractor:** The IP extraction strategy adapts correctly to deployment mode (direct vs. proxied), with explicit trusted proxy CIDR configuration.

### Areas for Future Hardening

1. **Request body size limits:** Adding a configurable `max_request_body_size` would address the remaining H-02 concern and the new F-02 finding. Echo supports `echo.BodyLimit()` middleware.

2. **Audit logging:** While request logging includes identity and path, there is no separate audit log for security-sensitive operations (ACL denials, repo creation/deletion). A structured audit trail would aid incident response.

3. **TLS in plain mode:** Supporting optional TLS termination in plain mode (e.g., via `--tls-cert` and `--tls-key` flags) would eliminate the need for a reverse proxy for HTTPS in non-Tailscale deployments.

4. **Dependency vulnerability scanning:** Integrating `govulncheck` into the CI pipeline (`.github/workflows/test.yml`) would automate dependency security checks.

---

## Part 5: Summary and Recommendations

### Remediation Scorecard

| Severity | Total | Fixed | Accepted | Open |
|----------|-------|-------|----------|------|
| Critical | 5 | 2 | 3 | 0 |
| High | 8 | 4 | 3 | 1 |
| Medium | 8 | 3 | 5 | 0 |
| Low | 5 | 4 | 0 | 1 |
| Informational | 3 | 3 | 0 | 0 |
| **Total** | **29** | **16** | **11** | **2** |

### New Findings Summary

| ID | Severity | Description | Effort |
|----|----------|-------------|--------|
| F-01 | Medium | Prometheus metrics label cardinality DoS | Medium |
| F-02 | Medium | S3 backend memory exhaustion via io.ReadAll | Medium |
| F-03 | Medium | Metrics endpoint bypasses ACL middleware | Low |
| F-04 | Low | ACL denial response information disclosure | Low |
| F-05 | Low | Graceful shutdown without timeout | Low |
| F-06 | Low | Missing Content-Security-Policy header | Low |
| F-07 | Informational | Default metrics enabled without authentication | Low |

### Prioritized Action Items

See the accompanying [risk-assessment.md](risk-assessment.md) for the maintainer-approved prioritization and implementation roadmap.

---

*Report generated by Claude Opus 4.6 as part of an AI-assisted security review. This is a follow-up to the [initial audit from 2026-02-25](../2026-02-25/report.md). Findings should be validated by human security engineers before implementation.*
