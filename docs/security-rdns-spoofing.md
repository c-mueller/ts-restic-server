# rDNS Spoofing: Attack Scenarios and Threat Model

This document assesses the security implications of using reverse DNS (rDNS) for identity resolution in ts-restic-server's plain mode. In Tailscale mode, the WhoIs API provides cryptographically authenticated identities and is not affected by these scenarios.

## Background

In plain mode (`listen_mode: plain`), the server resolves client IP addresses to hostnames via PTR record lookups (reverse DNS). These hostnames are then matched against ACL rules. Since DNS is inherently unauthenticated (unless DNSSEC is enforced end-to-end), an attacker who can manipulate PTR records may be able to spoof identities and bypass ACL restrictions.

The rDNS cache (default TTL: 600 seconds) amplifies the impact window: a single successful spoofing attempt grants the attacker 10 minutes of access without re-verification.

## Attack Scenarios

### Scenario 1: Same-LAN Attacker (ARP Spoofing + DNS)

**Prerequisites:**

- Attacker is on the same Layer 2 network as the restic server
- Network uses a local DNS server for PTR records (common in home/small office)
- ACL rules use hostname-based identities (e.g., `nas.home.arpa`)

**Attack Steps:**

1. Attacker ARP-spoofs to redirect DNS traffic or directly compromises the local DNS server
2. Attacker injects a PTR record mapping their IP to `nas.home.arpa`
3. Attacker connects to the restic server
4. Server performs rDNS lookup, resolves attacker's IP to `nas.home.arpa`
5. ACL matches the spoofed hostname, granting access

**Impact:** Full access to any repository that `nas.home.arpa` can access (read, write, delete depending on ACL rules).

**Likelihood:** **Medium-High** on unmanaged networks. ARP spoofing is trivial on flat L2 networks. Mitigated by managed switches with DHCP snooping and dynamic ARP inspection.

**Cache Amplification:** Once cached, the spoofed identity persists for the full TTL (default 10 minutes), even if the DNS is corrected.

### Scenario 2: Compromised DNS Server

**Prerequisites:**

- ACL uses a custom `dns_server` configuration pointing to a specific resolver
- Attacker gains control of that DNS server (or can intercept traffic to it)

**Attack Steps:**

1. Attacker compromises the configured DNS server (or performs DNS hijacking on the path)
2. Attacker configures PTR records to map any IP to a trusted hostname
3. Any client connecting to the restic server now resolves to the trusted hostname
4. ACL grants access based on the forged hostname

**Impact:** Complete bypass of hostname-based ACL rules. All clients appear as trusted identities.

**Likelihood:** **Low-Medium**. Requires compromising infrastructure. However, DNS traffic (port 53 UDP) is unencrypted and trivially intercepted on shared networks.

### Scenario 3: Upstream DNS Cache Poisoning

**Prerequisites:**

- Server uses system DNS (no custom `dns_server` configured)
- Attacker can send forged DNS responses to the upstream recursive resolver

**Attack Steps:**

1. Attacker performs a Kaminsky-style DNS cache poisoning attack against the recursive resolver
2. Attacker poisons the PTR record for their IP to resolve to `trusted-host.example.com`
3. Server's rDNS lookup returns the poisoned PTR record
4. ACL grants access

**Impact:** Same as Scenario 2, but affects all services using the poisoned resolver.

**Likelihood:** **Low**. Modern resolvers implement source port randomization and TXID entropy (RFC 5452). DNSSEC validation further mitigates this. However, not all resolvers validate DNSSEC.

### Scenario 4: Cross-Network PTR Record Control

**Prerequisites:**

- Attacker controls the authoritative DNS for the reverse zone of their IP range
- This is common for cloud providers, VPS hosts, and ISPs that delegate rDNS to customers

**Attack Steps:**

1. Attacker sets a PTR record for their VPS IP (e.g., `203.0.113.42`) to `nas.home.arpa`
2. Attacker connects to the restic server from this IP
3. Server performs rDNS, gets `nas.home.arpa` from the attacker-controlled authoritative DNS
4. ACL matches the hostname

**Impact:** Full access to repositories protected by hostname-based ACL rules.

**Likelihood:** **Medium** if the restic server is exposed to the internet. PTR records are controlled by the IP address owner (often the hosting provider or ISP), and many providers allow customers to set arbitrary PTR values. This is the most practical remote attack vector.

**Note:** Forward-confirmed reverse DNS (FCrDNS) — verifying that the returned hostname resolves back to the original IP — would mitigate this scenario but is not currently implemented.

## Summary Table

| Scenario | Prerequisites | Impact | Likelihood | Remote? |
|----------|--------------|--------|------------|---------|
| Same-LAN ARP spoofing | L2 access | High | Medium-High | No |
| Compromised DNS server | DNS control | Critical | Low-Medium | Depends |
| DNS cache poisoning | Resolver vuln | Critical | Low | Yes |
| Cross-network PTR control | IP ownership | High | Medium | Yes |

## Mitigations

### Currently Available

1. **Use Tailscale mode**: WhoIs provides cryptographically authenticated identities. This is the recommended approach for production deployments.

2. **Use IP-based ACL rules**: IP addresses are not affected by rDNS spoofing. Rules like `identities: ["10.0.0.5"]` are immune to DNS manipulation.

3. **Reduce cache TTL**: Lower `rdns_cache_ttl` to reduce the window of exploitation (at the cost of more DNS queries).

4. **Use a trusted local DNS server**: Configure `dns_server` to point to a trusted resolver on a secure network path.

5. **Network-level protections**: DHCP snooping, dynamic ARP inspection, and VLAN segmentation reduce same-LAN attack surface.

### Potential Future Mitigations

1. **Forward-confirmed rDNS (FCrDNS)**: After resolving IP → hostname via PTR, verify hostname → IP via A/AAAA. Only accept the hostname if both directions match. This blocks Scenario 4 (cross-network PTR control) and significantly raises the bar for other scenarios.

2. **DNS over TLS (DoT) / DNS over HTTPS (DoH)**: Encrypts and authenticates DNS queries to prevent interception and manipulation in transit. See feasibility assessment below.

3. **DNSSEC validation**: Verify DNSSEC signatures on DNS responses. Requires the DNS zone to be signed (not universally deployed).

## DoT/DoH Feasibility Assessment

### Technical Feasibility

**DNS over TLS (DoT)** and **DNS over HTTPS (DoH)** are both technically feasible to implement in Go:

- **`github.com/miekg/dns`**: Mature Go DNS library with DoT support (TLS transport). Used by CoreDNS and many production systems.
- **Standard `net/http`**: DoH can be implemented using standard HTTP client with JSON or wire-format DNS queries.
- **`github.com/ncruces/go-dns`**: Provides DoH resolver that integrates with Go's `net.Resolver`.

### Implementation Approach

The most practical approach would be a custom `net.Resolver` that uses DoT:

```go
resolver := &net.Resolver{
    PreferGo: true,
    Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
        d := tls.Dialer{Config: &tls.Config{ServerName: "dns.example.com"}}
        return d.DialContext(ctx, "tcp", "dns.example.com:853")
    },
}
```

This plugs into the existing `LookupAddr()` call with minimal code changes.

### Recommendation

**Do not implement DoT/DoH in ts-restic-server.** Instead, recommend users run a local encrypted DNS resolver.

**Rationale:**

1. **Complexity**: Adding DNS transport encryption increases code complexity and introduces new failure modes (TLS certificate validation, connection management, fallback behavior).

2. **Limited scope**: DoT/DoH only protects the DNS query path. It does not address the fundamental issue that PTR records are controlled by the IP address owner (Scenario 4) and that DNS itself is unauthenticated data.

3. **Better alternatives exist**: Local DNS resolvers like `unbound`, `stubby`, or `systemd-resolved` with DoT/DoH provide encrypted DNS for the entire system, not just this one application.

4. **Tailscale solves it properly**: For deployments requiring strong identity assurance, Tailscale mode provides cryptographic identity verification that no DNS-based approach can match.

**For users who need hostname-based ACL in plain mode:**

- Deploy a local DNS resolver with DoT/DoH (e.g., `unbound` with `forward-tls-upstream: yes`)
- Configure `dns_server` to point to the local resolver (e.g., `dns_server: "127.0.0.1:53"`)
- Consider adding IP-based rules as a fallback
- Reduce `rdns_cache_ttl` to limit exposure window

## Decision

**Accept the risk with clear documentation.** The rDNS trust model is inherently limited, and the recommended mitigations (Tailscale mode, IP-based rules, local encrypted resolver) adequately address the threat for most deployment scenarios. Implementing DoT/DoH in the application would add complexity without fundamentally solving the trust problem.
