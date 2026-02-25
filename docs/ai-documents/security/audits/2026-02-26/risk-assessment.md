# Risk Assessment — Follow-up Audit 2026-02-26

This document evaluates the new findings from the follow-up audit conducted on 2026-02-26 and the remaining open items from the [initial audit on 2026-02-25](../2026-02-25/report.md), incorporating the project maintainer's risk decisions and deployment context. The application is designed for internal/private network operation (primarily via Tailscale).

---

## Open Items from Original Audit (2026-02-25)

### H-05: rDNS Identity Spoofing in Plain Mode — REQUIRES INVESTIGATION

**Risk:** In plain mode with ACL enabled, identity resolution depends on reverse DNS (PTR records). An attacker controlling their DNS zone can set arbitrary PTR records to spoof identities matching ACL rules.

**Attack Scenario:**

1. Attacker determines the FQDN or hostname used in an ACL rule (e.g., via probing)
2. Attacker configures their PTR record to resolve to that hostname
3. Server performs rDNS on attacker's IP, gets the spoofed hostname
4. ACL grants access based on the spoofed identity

**Mitigating Factors:**

- Tailscale mode is not affected (authenticated WhoIs)
- IP address is always in the identity list — ACL rules matching on IP are not spoofable
- rDNS results include the FQDN (with domain), not just short hostname — matching requires exact FQDN
- Application is intended for internal/private network use only

**Recommendation:** Document the risk clearly in ACL documentation. Consider:

- Forward-confirmed reverse DNS (FCrDNS) validation
- DNS-over-TLS support for the rDNS resolver
- Warning at startup when ACL is enabled in plain mode without Tailscale

**Priority:** Medium — primarily a documentation and threat model concern for the intended deployment model.

### L-04: Docker Base Images Not Pinned — ACTION ITEM

**Risk:** Supply chain attack via compromised Docker base images.

**Current State:** Dockerfile uses mutable tags `golang:1.26-bookworm` and `debian:bookworm-slim`.

**Recommendation:** Pin to SHA256 digests:

```dockerfile
FROM golang:1.26-bookworm@sha256:<digest> AS build
FROM debian:bookworm-slim@sha256:<digest>
```

**Priority:** Low — theoretical risk for the project's scale, but a best practice.

---

## New Findings Assessment

### F-01: Prometheus Metrics Label Cardinality DoS — ACCEPTED RISK (Low Priority Enhancement)

**Risk Level:** Low (downgraded from Medium based on deployment context)

**Assessment:** The per-identity and per-repo-path label dimensions in `HostRequestsTotal`, `HostBytesReceivedTotal`, and `HostBytesSentTotal` create unbounded cardinality. In the intended Tailscale/internal deployment model, the number of unique identities is small and bounded by the network size.

**Maintainer Decision:** Accepted risk. The application is only operated internally, so the attack surface for label cardinality abuse is minimal.

**Optional Enhancement:** If the application is ever exposed to a broader network, implement one of:

- Make per-host metrics opt-in via configuration flag
- Cap tracked identities (e.g., top-N with "other" bucket)
- Use hash-bucketed labels to bound cardinality

**Priority:** Low — may be addressed opportunistically.

---

### F-02: S3 Backend Memory Exhaustion via io.ReadAll — ACTION ITEM (Low-Medium Priority)

**Risk Level:** Low-Medium

**Assessment:** `io.ReadAll()` in the S3 backend creates in-memory copies of every uploaded blob and config. While restic's default pack size is ~8MB, multiple concurrent uploads can compound memory usage. The risk is limited because write access is required to exploit this, and the application runs internally.

**Maintainer Decision:** Should be fixed. Higher priority than F-01 due to the potential for unintentional memory issues during heavy backup operations, regardless of the threat model.

**Action Required:**

- Short-term: Add `io.LimitReader(data, maxSize)` wrapper to cap individual blob size
- Long-term: Investigate streaming upload via S3's PutObject with `Content-Length`

**Priority:** Low-Medium — fix for robustness, not primarily for security.

---

### F-03: Metrics Endpoint Bypasses ACL — ACCEPTED RISK (Intentional, Low Priority Enhancement)

**Risk Level:** N/A (intentional design)

**Assessment:** The `/-/metrics` path is deliberately registered outside the ACL middleware chain. This is a feature: monitoring infrastructure (e.g., Prometheus scraper) needs access to metrics without being subject to repository-level ACL rules. The endpoint has its own separate Basic Auth mechanism.

**Maintainer Decision:** Accepted. This is intentional and the endpoint is secured separately from the data plane. The deployment environment provides additional network-level access control.

**Optional Enhancement:** Add an option to route the metrics endpoint through the ACL middleware for deployments that want unified access control.

**Priority:** Low — enhancement only if demand arises.

---

### F-04: ACL Denial Response Reveals Identity Resolution — ACCEPTED RISK (Low Priority Enhancement)

**Risk Level:** N/A (intentional design)

**Assessment:** The detailed identity information in ACL denial responses (hostname, user, tags) is an intentional debugging feature. The information disclosed is about the requesting client itself, not about other users or system internals.

**Maintainer Decision:** Accepted. Useful for operators troubleshooting access issues.

**Optional Enhancement:** Make identity disclosure in denial responses configurable via a config option (e.g., `acl.verbose_denials: true/false`) so it can be disabled in high-security deployments.

**Priority:** Low — enhancement only.

---

### F-05: Graceful Shutdown Without Timeout + Tailscale Cleanup — ACTION ITEM (High Priority)

**Risk Level:** Medium (upgraded based on maintainer observation)

**Assessment:** Two related shutdown issues have been identified:

1. **Unbounded shutdown context:** `server.go:73` uses `context.Background()` for `echo.Shutdown()`, meaning the process could hang indefinitely if any request handler blocks.

2. **Tailscale node not cleanly stopped:** The maintainer has observed that the Tailscale `tsnet.Server` is not always properly shut down during process termination. This can leave stale Tailscale nodes or orphaned state, requiring manual cleanup.

**Action Required:**

- Add a configurable shutdown timeout for the HTTP server (e.g., 30 seconds)
- Audit the Tailscale `tsnet.Server` lifecycle: verify `tsServer.Close()` is called reliably in all shutdown paths (signal, error, context cancellation)
- Test shutdown behavior under load and with active Tailscale connections
- Ensure deferred `tsServer.Close()` in `cmd/serve.go:86` executes in all exit scenarios

**Priority:** High — affects operational reliability and clean deployment lifecycle.

---

### F-06: Missing Content-Security-Policy Header — REJECTED (Not Applicable)

**Risk Level:** N/A

**Assessment:** This server is a REST API serving binary data and JSON responses. There is no frontend, no HTML rendering, and browsers are not the intended client. The `X-Content-Type-Options: nosniff` header already prevents MIME-type sniffing.

**Maintainer Decision:** Not applicable. Adding CSP provides no benefit for a pure API server and would be unnecessary overhead.

**No action required.**

---

### F-07: Default Metrics Enabled Without Authentication — ACCEPTED RISK (Intentional)

**Risk Level:** N/A (intentional design)

**Assessment:** Metrics are enabled by default without a password as a deliberate design choice. The application is intended for internal/private network operation where the network perimeter provides access control.

**Maintainer Decision:** Accepted. This is a feature for ease of deployment in trusted network environments.

**No action required.**

---

## Summary Table

| Finding | Severity | Decision | Action | Priority |
|---------|----------|----------|--------|----------|
| H-05 | Medium | Investigation | Document risks, evaluate FCrDNS | Medium |
| L-04 | Low | Action Item | Pin Docker images to SHA256 | Low |
| F-01 | Low | Accepted Risk | Optional cardinality controls | Low |
| F-02 | Low-Medium | Action Item | Add LimitReader / streaming upload | Low-Medium |
| F-03 | N/A | Accepted (Intentional) | Optional ACL integration | Low |
| F-04 | N/A | Accepted (Intentional) | Optional config toggle | Low |
| F-05 | Medium | **Action Item** | **Shutdown timeout + Tailscale cleanup** | **High** |
| F-06 | N/A | Rejected | None | N/A |
| F-07 | N/A | Accepted (Intentional) | None | N/A |

---

## Prioritized Implementation Roadmap

### Phase 1 — High Priority (Next Release)

1. **F-05: Shutdown Timeout + Tailscale Cleanup**
   - Add configurable shutdown timeout (default: 30s)
   - Audit and fix `tsnet.Server` cleanup in all shutdown paths
   - Test shutdown under load with active Tailscale connections
   - Verify `defer tsServer.Close()` reliability

### Phase 2 — Medium Priority (Near-Term)

1. **F-02: S3 Backend Request Body Limits**
   - Add `io.LimitReader` wrapper to `SaveBlob` and `SaveConfig`
   - Consider server-wide `max_request_body_size` config option

1. **H-05: rDNS Spoofing Documentation**
   - Document threat model for plain mode ACL
   - Evaluate FCrDNS validation feasibility

### Phase 3 — Low Priority (Backlog)

1. **L-04:** Pin Docker base images to SHA256 digests
1. **F-01:** Optional metrics cardinality controls (if needed)
1. **F-03:** Optional ACL integration for metrics endpoint
1. **F-04:** Optional config toggle for verbose ACL denials

---

## Overall Risk Summary

| Category | Count | Findings |
|----------|-------|----------|
| Action Items | 3 | F-02, F-05, L-04 |
| Investigation | 1 | H-05 |
| Accepted Risks (Intentional) | 3 | F-03, F-04, F-07 |
| Accepted Risks (Low Impact) | 1 | F-01 |
| Rejected (Not Applicable) | 1 | F-06 |

**Overall Assessment:** The application's security posture is strong for its intended deployment model (internal/Tailscale networks). The single high-priority item (F-05: shutdown reliability) is an operational concern rather than a security vulnerability. The remaining action items are robustness improvements that can be addressed incrementally.
