# Access Control Lists (ACL)

ts-restic-server provides fine-grained access control per identity and repository path. Without a configured ACL, all requests are allowed.

## Quick Start

```yaml
acl:
  default_role: deny
  rules:
    - paths: ["/server-a"]
      identities: ["server-a"]
      permission: full-access
    - paths: ["/"]
      identities: ["*"]
      permission: read-only
```

With this configuration, only the host `server-a` has full access to `/server-a`. All other clients get read-only access to all repositories.

## Concepts

### Identities

Each incoming request is associated with a list of **identities**. These vary by mode:

| Mode | Identities | Example |
|---|---|---|
| Tailscale (WhoIs) | IP, FQDN, short hostname, login name, tags | `["100.64.0.1", "server.zuul-vibes.ts.net", "server", "alice@example.com", "tag:backup"]` |
| Plain (with rDNS) | IP, FQDN | `["10.0.0.5", "nas.home.arpa"]` |
| Plain (no rDNS / error) | IP only | `["10.0.0.5"]` |

An ACL rule matches when **any** identity of the request matches **any** identity of the rule (N:M matching).

### Permissions

There are four permission levels (ascending):

| Permission | Read | Write | Delete |
|---|---|---|---|
| `deny` | - | - | - |
| `read-only` | yes | - | - |
| `append-only` | yes | yes | - |
| `full-access` | yes | yes | yes |

**Operations and HTTP methods:**

| Operation | HTTP Methods | Minimum Permission |
|---|---|---|
| Read | `GET`, `HEAD` | `read-only` |
| Write | `POST`, `PUT`, `PATCH`, `DELETE /locks/*` | `append-only` |
| Delete | `DELETE` (blobs) | `full-access` |

Lock deletion (`DELETE /locks/*`) is treated as a write operation, not a delete. This allows append-only clients to manage their own locks.

### Path Matching

Rule paths match on segment boundaries as a prefix:

- `/server-a` matches `/server-a` and `/server-a/sub/path`
- `/server-a` does **not** match `/server-ab` (no segment boundary)
- `/` matches everything

### Cascading

When multiple rules apply to a request:

1. **Deepest path wins**: A rule on `/server-a` takes precedence over a rule on `/`.
2. **Deny is absolute**: If a deny rule exists at the deepest matching level, access is denied regardless of other rules at the same level.
3. **Highest permission wins**: If no deny is present, the highest permission at the deepest matching level applies.
4. **Default role as fallback**: If no rule matches, `default_role` is used.

**Example:**

```yaml
acl:
  default_role: deny
  rules:
    - paths: ["/"]
      identities: ["*"]
      permission: read-only      # level 0: everyone can read
    - paths: ["/server-a"]
      identities: ["server-a"]
      permission: full-access    # level 1: server-a has full access
    - paths: ["/private"]
      identities: ["*"]
      permission: deny           # level 1: /private is locked
```

| Client | Path | Result | Reason |
|---|---|---|---|
| `server-a` | `/server-a` | `full-access` | Rule at level 1 matches |
| `server-a` | `/other` | `read-only` | Fallback to level 0 (`/`) |
| `server-b` | `/server-a` | `read-only` | Identity doesn't match at level 1, fallback to level 0 |
| `*` | `/private` | `deny` | Deny at level 1 |
| `server-x` | `/unknown` | `deny` | No rule matches, `default_role: deny` |

## Identity Resolution

### Tailscale Mode (`listen_mode: tailscale`) — WhoIs

In Tailscale mode, identities are resolved via the **Tailscale WhoIs API** (`tsnet.Server.LocalClient().WhoIs()`). This provides significantly more information than rDNS:

- **IP** — Tailscale IP (e.g. `100.64.0.1`)
- **FQDN** — Tailscale hostname (e.g. `server.zuul-vibes.ts.net`)
- **Short hostname** — ComputedName of the node (e.g. `server`)
- **Login name** — Tailscale user (e.g. `alice@example.com`)
- **Tags** — Tailscale ACL tags (e.g. `tag:server`, `tag:backup`)

This enables rules based on tags and users:

```yaml
rules:
  - paths: ["/tagged-servers"]
    identities: ["tag:server"]
    permission: full-access
  - paths: ["/alice-backup"]
    identities: ["alice@example.com"]
    permission: full-access
```

If `LocalClient()` fails, the system automatically falls back to rDNS via MagicDNS.

### Plain Mode (`listen_mode: plain`)

- rDNS queries go to the **system DNS** or a configured `dns_server`
- Resolution yields the FQDN (e.g. `nas.home.arpa`)
- **No** short hostname (ambiguous in plain mode)

### Graceful Degradation

If a WhoIs or rDNS lookup fails (timeout, NXDOMAIN, no PTR record), only the IP is used as identity. The server does not reject the request — ACL rules are evaluated with the IP as the sole identity.

### Cache

Identity results (WhoIs and rDNS) are cached to avoid repeated lookups per request:

- **Default TTL:** 600 seconds (10 minutes)
- **Negative results** (no PTR record, errors) are also cached
- TTL is configurable via `rdns_cache_ttl`

## Configuration

### Full Reference

```yaml
acl:
  default_role: deny              # fallback permission when no rule matches
  dns_server: ""                  # custom DNS server for rDNS (host:port)
  rdns_cache_ttl: 600             # cache TTL in seconds (default: 600)
  trusted_proxies:                # CIDRs of trusted reverse proxies
    - 10.0.0.0/8
  rules:
    - paths: ["/server-a"]
      identities: ["server-a"]
      permission: full-access
```

### Config Options

| Option | Type | Default | Description |
|---|---|---|---|
| `default_role` | string | (required) | `deny`, `read-only`, `append-only`, or `full-access` |
| `dns_server` | string | `""` | Custom DNS server in `host:port` format. Empty = system DNS. In Tailscale mode, WhoIs is used (fallback: MagicDNS `100.100.100.100:53`). |
| `rdns_cache_ttl` | int | `600` | TTL for identity cache in seconds |
| `trusted_proxies` | []string | `[]` | CIDR notations of trusted proxies for `X-Forwarded-For` |
| `rules` | []Rule | `[]` | List of ACL rules |

### Rule Options

| Option | Type | Description |
|---|---|---|
| `paths` | []string | Repository paths the rule applies to (prefix match) |
| `identities` | []string | Identities to match. `"*"` = all. |
| `permission` | string | `deny`, `read-only`, `append-only`, or `full-access` |

### Disabling ACL

The ACL is disabled when the entire `acl:` block is absent from the config. Without ACL, all requests are allowed.

## Trusted Proxies

When the server runs behind a reverse proxy (Nginx, Caddy, etc.), the client IP must be extracted from the `X-Forwarded-For` header. For this, the proxy IPs must be configured as `trusted_proxies`:

```yaml
acl:
  default_role: read-only
  trusted_proxies:
    - 10.0.0.0/8
    - 172.16.0.0/12
  rules: [...]
```

| Configuration | Behavior |
|---|---|
| Tailscale mode | Direct connection, no proxy headers, `trusted_proxies` is ignored |
| Plain, no `trusted_proxies` | Echo defaults (loopback + link-local + private nets are trusted) |
| Plain, with `trusted_proxies` | Only the specified CIDRs are trusted as proxies |

## Examples

### Tailscale: One Repo per Host

Each Tailscale host gets its own repository with full access. All others can read.

```yaml
listen_mode: tailscale

acl:
  default_role: read-only
  rules:
    - paths: ["/server-a"]
      identities: ["server-a"]
      permission: full-access
    - paths: ["/server-b"]
      identities: ["server-b"]
      permission: full-access
    - paths: ["/laptop"]
      identities: ["laptop"]
      permission: full-access
```

### Tailscale: Tag-based Access

All nodes with the `tag:backup` tag get full access.

```yaml
listen_mode: tailscale

acl:
  default_role: deny
  rules:
    - paths: ["/"]
      identities: ["tag:backup"]
      permission: full-access
```

### Tailscale: User-based Access

Specific Tailscale users get access to dedicated repos.

```yaml
listen_mode: tailscale

acl:
  default_role: deny
  rules:
    - paths: ["/alice"]
      identities: ["alice@example.com"]
      permission: full-access
    - paths: ["/bob"]
      identities: ["bob@example.com"]
      permission: full-access
    - paths: ["/"]
      identities: ["*"]
      permission: read-only
```

### Tailscale: Shared Repo with FQDN

```yaml
listen_mode: tailscale

acl:
  default_role: deny
  rules:
    - paths: ["/team-backup"]
      identities:
        - "server-a.zuul-vibes.ts.net"
        - "server-b.zuul-vibes.ts.net"
      permission: full-access
    - paths: ["/team-backup"]
      identities: ["*"]
      permission: read-only
```

### Plain: IP-based with Reverse Proxy

```yaml
listen_mode: plain

acl:
  default_role: deny
  trusted_proxies:
    - 10.0.0.1/32
  rules:
    - paths: ["/nas"]
      identities: ["192.168.1.100"]
      permission: full-access
    - paths: ["/"]
      identities: ["*"]
      permission: read-only
```

### Plain: Hostname-based with Custom DNS

```yaml
listen_mode: plain

acl:
  default_role: deny
  dns_server: "192.168.1.1:53"
  rdns_cache_ttl: 300
  rules:
    - paths: ["/nas"]
      identities: ["nas.home.arpa"]
      permission: full-access
    - paths: ["/"]
      identities: ["*"]
      permission: read-only
```

### Append-Only Backup

Clients can write but not delete — ideal for ransomware protection:

```yaml
acl:
  default_role: deny
  rules:
    - paths: ["/"]
      identities: ["*"]
      permission: append-only
```

> **Note:** For global append-only mode without identity-based differentiation, you can alternatively set `append_only: true` at the top level. The ACL provides finer control per identity and path.

## rDNS Trust Model and Security Considerations

In **plain mode**, identity resolution relies on reverse DNS (PTR records), which is inherently unauthenticated. This has important security implications:

**Key assumptions:**

- rDNS responses are only as trustworthy as the DNS infrastructure between the server and the authoritative nameserver for the reverse zone
- PTR records for a given IP are controlled by the owner of that IP range, not the domain owner — an attacker controlling their own IP's PTR record can set it to any hostname
- DNS queries (port 53/UDP) are unencrypted and can be intercepted or manipulated on shared networks
- Cached results (default TTL: 600s) amplify the window of any successful spoofing attack

**Recommendations for production deployments:**

1. **Prefer Tailscale mode** for strong identity assurance (cryptographic verification via WhoIs API)
2. **Use IP-based ACL rules** when possible — these are immune to DNS manipulation
3. **Run a local encrypted DNS resolver** (e.g., `unbound` with DoT) and point `dns_server` to it
4. **Reduce `rdns_cache_ttl`** in sensitive environments to limit exposure windows

For a detailed threat analysis including attack scenarios and a DoT/DoH feasibility assessment, see [docs/security-rdns-spoofing.md](security-rdns-spoofing.md).

## Error Response on Access Denial

When a request is denied by the ACL, the server responds with `403 Forbidden` and a JSON body containing the requester's identity. This allows the client to understand why access was refused.

**Tailscale mode** (with WhoIs resolution):

```json
{
  "error": "access denied",
  "path": "/server-a",
  "operation": "write",
  "ip": "100.64.0.1",
  "hostname": "server.zuul-vibes.ts.net",
  "user": "alice@example.com",
  "tags": ["tag:backup"]
}
```

**Plain mode** (IP only):

```json
{
  "error": "access denied",
  "path": "/server-a",
  "operation": "write",
  "ip": "10.0.0.5"
}
```

Fields like `hostname`, `user`, and `tags` only appear when available through WhoIs resolution.

## Logging

### Access Log

The HTTP access log includes an `identities` field when identity resolution yielded more than just the IP (e.g. hostname, tags, user). When only the IP is available, the field is omitted.

### ACL Denied Log

On access denials, the ACL middleware logs an `acl denied` warning with `request_id`, `identities`, `repo_path`, `operation`, `method`, and `path`. The `request_id` enables correlation with the access log.

## Middleware Order

The request pipeline looks like this:

```text
Request → RepoPrefix → Recover → RequestID → Logger → Identity → ACL → Handler
```

1. **RepoPrefix** extracts the repo path prefix and rewrites the URL
2. **Identity** resolves the client IP via WhoIs (Tailscale) or rDNS (plain) and stores the identities in the context
3. **ACL** reads the identities from the context and checks against the rules

## Architecture

### Relevant Files

| File | Description |
|---|---|
| `internal/config/config.go` | `ACLConfig` struct, validation |
| `internal/acl/acl.go` | ACL engine: rule evaluation, path and identity matching |
| `internal/acl/acl_test.go` | Unit tests for the ACL engine |
| `internal/middleware/identity.go` | WhoIs and rDNS resolver, cache, identity middleware |
| `internal/middleware/acl.go` | ACL middleware: reads identities, enforces permissions, JSON error response |
| `internal/middleware/logger.go` | HTTP access log with identity field |
| `internal/server/server.go` | Server struct with optional `*tsnet.Server` |
| `internal/server/listener.go` | Accepts external `*tsnet.Server` |
| `cmd/serve.go` | Wiring: tsnet creation, WhoIs adapter, `buildACLEngine`, `buildIPExtractor`, `buildIdentityMiddleware` |
