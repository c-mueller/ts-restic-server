# Risk Assessment — ts-restic-server

| Field          | Value                              |
|----------------|------------------------------------|
| Date           | 2026-02-25                         |
| Basis          | Security Audit Report 2026-02-25   |
| Assessed by    | Project Maintainer                 |

This document provides a risk assessment for each finding from the security audit dated 2026-02-25. Findings are categorized into action items, accepted risks, and items requiring further investigation.

---

## Action Items

These findings will be addressed through code changes.

### C-04: Path Traversal in Repo Prefix Middleware — FIX

Risk confirmed. Missing validation of `..` and `.` segments in the repo prefix middleware is a legitimate vulnerability. The fix is straightforward (reject path segments containing traversal sequences) and should be implemented promptly.

Action: Add path segment validation in `internal/middleware/repoprefix.go`.

---

### C-05: Range Header Allows Negative Values — FIX

Risk confirmed. Negative offsets in `parseRange()` are invalid and must be rejected. This is a low-effort fix with no impact on legitimate clients.

Action: Add bounds check in `internal/api/blob.go:parseRange()` to reject negative start/end values and ensure `start <= end`.

---

### H-01: Blob Name Sanitization — IMPLEMENT

Risk confirmed. Restic blob names are SHA-based hex strings (e.g., `a3f2b8c1...`). A small number of special-case names exist (e.g., lock IDs), but all legitimate names follow predictable patterns. Input validation against an expected format (hex characters only, bounded length) is both feasible and advisable.

Action: Implement blob name validation in `blobParams()`. Reject names containing `/`, `\`, `..`, null bytes, or non-hex characters. Verify the exact naming rules against the restic REST API specification before implementing.

---

### H-04: ACL Bypass on Empty Identity — FIX

Risk critical and confirmed. If the identity middleware returns an empty slice `[]string{}` instead of `nil`, the ACL middleware skips the fallback to `c.RealIP()`, passing an empty identity list to the ACL engine. Depending on rule configuration this could result in incorrect allow/deny decisions.

Action: Change the nil check in `internal/middleware/acl.go` to also handle empty slices:

```go
if identities == nil || len(identities) == 0 {
    identities = []string{c.RealIP()}
}
```

---

### M-03: Unbounded Identity Cache — LIMIT

Risk confirmed. An unbounded `map[string]` cache can grow indefinitely if requests arrive from many distinct IPs. This is a realistic DoS vector in plain mode.

Action: Introduce a bounded cache with a default maximum of 1000 entries and FIFO eviction (oldest entries are evicted first when the limit is reached). Make the limit configurable.

---

### L-01: Accept Header Case Sensitivity — FIX

Risk low but valid. HTTP media types should be compared case-insensitively. The fix is trivial.

Action: Normalize the Accept header to lowercase before comparison in `internal/api/version.go`.

---

### L-04: Docker Base Images Not Pinned — FIX

Risk low but valid (supply chain). Mutable tags could be replaced upstream. Pinning to SHA256 digests is best practice.

Action: Pin base images in `Dockerfile` to specific digests.

---

### L-02: Inconsistent HTTP Status Codes — EVALUATE

Risk low. Inconsistent error responses across handlers can confuse clients and complicate debugging.

Action: Review all handlers and standardize error response format. Consider a consistent JSON error envelope.

---

### I-01: ListBlobs Basename in Sharded Mode — VERIFY

Action: Verify that returning `info.Name()` matches the restic REST API specification for sharded data directories. If clients expect bare blob names (without shard prefix), the current behavior may be correct — but this needs explicit confirmation.

---

### I-02: ACL Path Matching vs. Symlink Resolution — INVESTIGATE

Action: Evaluate whether symlink resolution in the filesystem backend can produce paths that diverge from the logical paths used in ACL rules. Document the expected behavior.

---

### I-03: Panic Recovery Logs Full Panic Values — REVIEW

Action: Assess whether panic values could contain sensitive data (credentials, user content). If so, limit logging to panic type and a sanitized message.

---

## Accepted Risks

These findings have been evaluated and are accepted as-is, with documented reasoning.

### C-01: No Built-in Authentication — ACCEPTED

Assessment: This is by design, not a vulnerability.

Authentication is intentionally delegated to the surrounding infrastructure:

- **Tailscale mode:** Tailscale provides mutual authentication, encrypted transport, and identity verification (WhoIs). The built-in ACL engine enforces per-identity access control on top of this. This is the recommended deployment model.
- **Plain mode with ACL:** IP-based ACL rules provide coarse access control. This mode is intended for trusted networks or deployments behind a reverse proxy that handles authentication (e.g., nginx with Basic Auth, OAuth2 Proxy, Cloudflare Access).
- **Plain mode without ACL:** This is intentionally an open configuration for local/development use.

This design philosophy is consistent across the application and its documentation. Adding a built-in authentication layer would duplicate functionality already provided by the infrastructure layer and increase maintenance surface.

Mitigation already in place: The application documents that plain mode without additional protection is unsuitable for untrusted networks.

Further action: Add a prominent note in the README security section reinforcing this.

---

### C-02: Unencrypted Plain HTTP by Default — ACCEPTED

Assessment: This is an intentional default for development and local use.

Plain HTTP is the default because:

- Tailscale mode provides TLS automatically — this is the recommended production deployment.
- In plain mode, TLS termination is expected at the reverse proxy layer.
- Requiring TLS configuration for a local development server would create unnecessary friction.

Further action: Add a note in the README stating that the default configuration is not suitable for production exposure on untrusted networks and that Tailscale mode or a TLS-terminating reverse proxy should be used.

---

### C-03: Default Listen Address 0.0.0.0 — ACCEPTED

Assessment: This is a standard server default.

Binding to all interfaces is the conventional default for server applications. Restricting to `127.0.0.1` would break Docker and Tailscale deployments out of the box.

Further action: Same as C-02 — document secure deployment practices in README.

---

### H-02: Unbounded Request Bodies — ACCEPTED (Low Risk)

Assessment: This is tolerable given the deployment model.

This application is not a public-facing web service. It serves a restricted set of restic clients in controlled environments (Tailscale networks, private infrastructure). Restic itself limits blob sizes by design (data blobs are typically 512 KiB to 8 MiB).

Introducing a hard request body limit risks breaking legitimate large uploads (e.g., large snapshot metadata or pack files) and adds configuration complexity.

Residual risk: A malicious client within the trusted network could send an oversized request to exhaust server memory. This is accepted because:

- Access to the server already implies network-level trust.
- The memory backend has its own quota enforcement (albeit post-read).
- The filesystem and S3 backends stream to disk/remote storage without full buffering.

Optional future improvement: A configurable maximum body size could be added as a defense-in-depth measure, defaulting to a generous limit (e.g., 128 MiB).

---

### H-03: Credentials in Plaintext Configuration — ACCEPTED (with improvements)

Assessment: This is inherent to configuration-based credential management.

There is no practical alternative to storing backend credentials in the configuration when static credentials are required (S3 access keys, WebDAV passwords, etc.). The responsibility for securing the configuration file lies with the operator (file permissions, encrypted storage, secrets management).

Accepted as-is, with two planned improvements:

1. **Separate ACL from main configuration:** Extract ACL rules into a dedicated file to reduce the sensitivity of the main config file and allow independent access control on each file.

1. **Environment variable substitution in config values:** Allow referencing environment variables within YAML config values, e.g.:

   ```yaml
   tailscale:
     auth_key: ${TS_AUTH_KEY}
   storage:
     s3:
       secret_key: ${AWS_SECRET_ACCESS_KEY}
   ```

   At runtime, `${VAR_NAME}` expressions would be resolved from the process environment. This enables integration with secret injection systems (Kubernetes secrets, Docker secrets, systemd credentials, HashiCorp Vault agent) without changing the config file format.

   Note: Direct environment variable support already exists via Viper (`RESTIC_STORAGE_S3_SECRET_KEY` etc.), but in-config substitution provides more flexibility for operators managing multiple credential sources.

---

### M-08: No Rate Limiting — ACCEPTED

Assessment: This is not applicable for the intended deployment model.

Rate limiting is a concern for public-facing services. ts-restic-server operates in controlled environments:

- **Tailscale:** Access is inherently limited to authenticated nodes on the tailnet. Abuse from within the tailnet is not a realistic threat model.
- **Plain mode:** If deployed behind a reverse proxy, rate limiting should be configured at the proxy layer (nginx `limit_req`, HAProxy rate limiting, etc.).

Implementing rate limiting in the application itself would add complexity without meaningful security benefit for the intended use cases.

---

## Findings Requiring Further Investigation

### H-05: rDNS Identity Spoofing — INVESTIGATE

Assessment: This is theoretically possible but practically difficult.

In plain mode, rDNS-based identity resolution is inherently weaker than Tailscale's WhoIs mechanism. An attacker who controls their PTR records could spoof a hostname that matches an ACL rule.

In Tailscale mode this is not a concern. Identity is resolved via the Tailscale control plane (100.100.100.100), which is authenticated and encrypted. Interception of WhoIs responses is unrealistic in practice.

In plain mode the risk depends on the DNS infrastructure:

- If the DNS server is on a trusted network, spoofing requires compromising the DNS server itself.
- If DNS queries traverse untrusted networks, spoofing becomes easier.

Required actions:

1. **Document threat model:** Describe the rDNS trust assumptions explicitly in the ACL documentation.
1. **Develop attack scenarios:** Work out concrete attack scenarios for rDNS spoofing in both deployment modes to quantify the actual risk.
1. **Evaluate authenticated DNS support:** Assess the feasibility of supporting DNS-over-TLS (DoT) or DNS-over-HTTPS (DoH) for rDNS lookups. This would not be enabled by default but would allow security-conscious operators to harden their deployment.

---

### M-01: Filesystem Backend Follows Symlinks — INVESTIGATE

Assessment: This needs evaluation.

It is unclear whether symlink following is a practical concern in the intended deployment. The filesystem backend's data directory is typically managed exclusively by ts-restic-server; external symlink creation would require write access to the data directory.

Action: Evaluate whether `filepath.EvalSymlinks()` or `os.Lstat` checks should be added. Consider the performance impact of additional syscalls per file operation.

---

### M-02: Atomic Write Race Condition — INVESTIGATE

Assessment: This needs evaluation.

The TOCTOU window in `atomicWrite` is narrow and only exploitable under concurrent write access to the same blob path. Restic's content-addressed storage model means that concurrent writes to the same name would contain identical data — but this assumption should be verified.

Action: Analyze whether concurrent writes to the same blob can occur in practice and whether the current implementation handles them safely.

---

## Summary

| Finding | Decision | Priority |
|---------|----------|----------|
| C-01: No authentication | Accepted (by design) | — |
| C-02: No TLS default | Accepted (document in README) | Low |
| C-03: Default bind 0.0.0.0 | Accepted (document in README) | Low |
| C-04: Path traversal in repo prefix | **Fix** | High |
| C-05: Negative range values | **Fix** | High |
| H-01: Blob name sanitization | **Implement** | High |
| H-02: Unbounded request bodies | Accepted (low risk) | — |
| H-03: Plaintext credentials | Accepted (with improvements planned) | Medium |
| H-04: ACL empty identity bypass | **Fix** | Critical |
| H-05: rDNS spoofing | Investigate (attack scenarios) | Medium |
| H-06: Missing security headers | **Implement** | Medium |
| H-07: Backend credentials over HTTP | Accepted (config validation exists) | — |
| H-08: Blob type case sensitivity | **Fix** | Low |
| M-01: Symlink following | Investigate | Medium |
| M-02: Race condition | Investigate | Low |
| M-03: Unbounded identity cache | **Fix** (1000 entry FIFO limit) | Medium |
| M-04: Wildcard ACL permissiveness | Accepted (document behavior) | — |
| M-05: Append-only incomplete | Accepted (document limitations) | — |
| M-06: S3 error string matching | Accepted (defensive fallback) | — |
| M-07: Query string logging | Accepted (no sensitive params currently) | — |
| M-08: No rate limiting | Accepted (infra responsibility) | — |
| L-01: Accept header case | **Fix** | Low |
| L-02: Inconsistent status codes | **Evaluate** | Low |
| L-04: Docker image pinning | **Fix** | Low |
| L-05: Panic recovery partial response | **Review** | Low |
| I-01: ListBlobs sharded basename | **Verify** | Low |
| I-02: ACL vs. symlinks | **Investigate** | Low |
| I-03: Panic log verbosity | **Review** | Low |

### Implementation priority order

1. H-04 — ACL empty identity bypass (critical correctness bug)
1. C-04 — Path traversal in repo prefix
1. C-05 — Negative range values
1. H-01 — Blob name validation
1. M-03 — Bounded identity cache (1000 FIFO)
1. H-06 — Security response headers
1. H-03 improvements — Config/ACL separation, env var substitution
1. H-05 — rDNS threat model documentation and DoT/DoH evaluation
1. L-01, L-04, H-08 — Minor fixes (case sensitivity, Docker pinning, blob type normalization)
1. L-02, L-05, I-01–I-03 — Review and verify
