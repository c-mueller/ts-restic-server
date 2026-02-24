# Access Control Lists (ACL)

ts-restic-server bietet eine feingranulare Zugriffskontrolle per Identity und Repository-Pfad. Ohne konfigurierte ACL werden alle Requests zugelassen.

## Schnellstart

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

Mit dieser Konfiguration hat nur der Host `server-a` vollen Zugriff auf `/server-a`. Alle anderen Clients bekommen lesenden Zugriff auf alle Repositories.

## Konzepte

### Identities

Jeder eingehende Request wird einer Liste von **Identities** zugeordnet. Diese bestehen je nach Modus aus:

| Modus | Identities | Beispiel |
|---|---|---|
| Tailscale (WhoIs) | IP, FQDN, Short-Hostname, Login-Name, Tags | `["100.64.0.1", "server.zuul-vibes.ts.net", "server", "alice@example.com", "tag:backup"]` |
| Plain (mit rDNS) | IP, FQDN | `["10.0.0.5", "nas.home.arpa"]` |
| Plain (ohne rDNS / Fehler) | nur IP | `["10.0.0.5"]` |

Eine ACL-Regel matcht, sobald **eine beliebige** Identity des Requests mit **einer beliebigen** Identity der Regel übereinstimmt (N:M-Matching).

### Permissions

Es gibt vier Berechtigungsstufen (aufsteigend):

| Permission | Lesen | Schreiben | Löschen |
|---|---|---|---|
| `deny` | - | - | - |
| `read-only` | ja | - | - |
| `append-only` | ja | ja | - |
| `full-access` | ja | ja | ja |

**Operationen und HTTP-Methoden:**

| Operation | HTTP-Methoden | Mindest-Permission |
|---|---|---|
| Read | `GET`, `HEAD` | `read-only` |
| Write | `POST`, `PUT`, `PATCH`, `DELETE /locks/*` | `append-only` |
| Delete | `DELETE` (Blobs) | `full-access` |

Lock-Deletion (`DELETE /locks/*`) wird als Write-Operation gewertet, nicht als Delete. Das ermöglicht append-only-Clients, ihre eigenen Locks zu verwalten.

### Pfad-Matching

Regel-Pfade matchen auf Segment-Grenzen als Prefix:

- `/server-a` matcht `/server-a` und `/server-a/sub/path`
- `/server-a` matcht **nicht** `/server-ab` (keine Segment-Grenze)
- `/` matcht alles

### Kaskadierung

Wenn mehrere Regeln auf einen Request zutreffen:

1. **Tiefster Pfad gewinnt**: Eine Regel auf `/server-a` hat Vorrang vor einer Regel auf `/`.
2. **Deny ist absolut**: Wenn auf der tiefsten Matching-Ebene eine Deny-Regel existiert, wird der Zugriff verweigert, unabhängig von anderen Regeln auf derselben Ebene.
3. **Höchste Permission gewinnt**: Wenn kein Deny vorliegt, gilt die höchste Permission auf der tiefsten Matching-Ebene.
4. **Default-Role als Fallback**: Matcht keine Regel, greift `default_role`.

**Beispiel:**

```yaml
acl:
  default_role: deny
  rules:
    - paths: ["/"]
      identities: ["*"]
      permission: read-only      # Ebene 0: alle dürfen lesen
    - paths: ["/server-a"]
      identities: ["server-a"]
      permission: full-access    # Ebene 1: server-a hat vollen Zugriff
    - paths: ["/private"]
      identities: ["*"]
      permission: deny           # Ebene 1: /private ist gesperrt
```

| Client | Pfad | Ergebnis | Grund |
|---|---|---|---|
| `server-a` | `/server-a` | `full-access` | Regel auf Ebene 1 matcht |
| `server-a` | `/other` | `read-only` | Fallback auf Ebene 0 (`/`) |
| `server-b` | `/server-a` | `read-only` | Identity matcht nicht auf Ebene 1, Fallback auf Ebene 0 |
| `*` | `/private` | `deny` | Deny auf Ebene 1 |
| `server-x` | `/unknown` | `deny` | Keine Regel matcht, `default_role: deny` |

## Identity-Auflösung

### Tailscale-Modus (`listen_mode: tailscale`) — WhoIs

Im Tailscale-Modus werden Identities über die **Tailscale WhoIs-API** (`tsnet.Server.LocalClient().WhoIs()`) aufgelöst. Das liefert deutlich mehr Informationen als rDNS:

- **IP** — Tailscale-IP (z.B. `100.64.0.1`)
- **FQDN** — Tailscale-Hostname (z.B. `server.zuul-vibes.ts.net`)
- **Short-Hostname** — ComputedName des Nodes (z.B. `server`)
- **Login-Name** — Tailscale-User (z.B. `alice@example.com`)
- **Tags** — Tailscale ACL-Tags (z.B. `tag:server`, `tag:backup`)

Dies ermöglicht Regeln basierend auf Tags und Usern:

```yaml
rules:
  - paths: ["/tagged-servers"]
    identities: ["tag:server"]
    permission: full-access
  - paths: ["/alice-backup"]
    identities: ["alice@example.com"]
    permission: full-access
```

Falls `LocalClient()` fehlschlägt, wird automatisch auf rDNS via MagicDNS zurückgefallen.

### Plain-Modus (`listen_mode: plain`)

- rDNS-Queries gehen an den **System-DNS** oder einen konfigurierten `dns_server`
- Auflösung ergibt den FQDN (z.B. `nas.home.arpa`)
- **Kein** Short-Hostname (da im Plain-Modus mehrdeutig)

### Graceful Degradation

Schlägt ein rDNS-Lookup fehl (Timeout, NXDOMAIN, kein PTR-Record), wird nur die IP als Identity verwendet. Der Server lehnt den Request nicht ab — die ACL-Regeln werden mit der IP als einziger Identity ausgewertet.

### Cache

rDNS-Ergebnisse werden gecached um wiederholte Lookups pro Request zu vermeiden:

- **Default-TTL:** 600 Sekunden (10 Minuten)
- **Negative Ergebnisse** (kein PTR-Record, Fehler) werden ebenfalls gecached
- TTL ist konfigurierbar über `rdns_cache_ttl`

## Konfiguration

### Vollständige Referenz

```yaml
acl:
  default_role: deny              # Fallback-Permission wenn keine Regel matcht
  dns_server: ""                  # Custom DNS-Server für rDNS (host:port)
  rdns_cache_ttl: 600             # Cache-TTL in Sekunden (Default: 600)
  trusted_proxies:                # CIDRs vertrauenswürdiger Reverse-Proxies
    - 10.0.0.0/8
  rules:
    - paths: ["/server-a"]
      identities: ["server-a"]
      permission: full-access
```

### Config-Optionen

| Option | Typ | Default | Beschreibung |
|---|---|---|---|
| `default_role` | string | (pflicht) | `deny`, `read-only`, `append-only` oder `full-access` |
| `dns_server` | string | `""` | Custom DNS-Server im Format `host:port`. Leer = System-DNS. Im Tailscale-Modus wird automatisch `100.100.100.100:53` verwendet. |
| `rdns_cache_ttl` | int | `600` | TTL für rDNS-Cache in Sekunden |
| `trusted_proxies` | []string | `[]` | CIDR-Notationen vertrauenswürdiger Proxies für `X-Forwarded-For` |
| `rules` | []Rule | `[]` | Liste von ACL-Regeln |

### Regel-Optionen

| Option | Typ | Beschreibung |
|---|---|---|
| `paths` | []string | Repo-Pfade auf die die Regel zutrifft (Prefix-Match) |
| `identities` | []string | Identities die matchen sollen. `"*"` = alle. |
| `permission` | string | `deny`, `read-only`, `append-only` oder `full-access` |

### ACL deaktivieren

Die ACL ist deaktiviert wenn der gesamte `acl:`-Block in der Config fehlt. Ohne ACL werden alle Requests zugelassen.

## Trusted Proxies

Wenn der Server hinter einem Reverse-Proxy läuft (Nginx, Caddy, etc.), muss die Client-IP aus dem `X-Forwarded-For`-Header extrahiert werden. Dafür müssen die Proxy-IPs als `trusted_proxies` konfiguriert werden:

```yaml
acl:
  default_role: read-only
  trusted_proxies:
    - 10.0.0.0/8
    - 172.16.0.0/12
  rules: [...]
```

| Konfiguration | Verhalten |
|---|---|
| Tailscale-Modus | Direkte Verbindung, keine Proxy-Header, `trusted_proxies` wird ignoriert |
| Plain, keine `trusted_proxies` | Echo-Defaults (Loopback + Link-Local + private Netze werden vertraut) |
| Plain, mit `trusted_proxies` | Nur die angegebenen CIDRs werden als Proxy vertraut |

## Beispiele

### Tailscale: Ein Repo pro Host

Jeder Tailscale-Host bekommt ein eigenes Repository mit vollem Zugriff. Alle anderen können lesen.

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

### Tailscale: Tag-basierter Zugriff

Alle Nodes mit dem Tag `tag:backup` bekommen vollen Zugriff auf ihre Repos.

```yaml
listen_mode: tailscale

acl:
  default_role: deny
  rules:
    - paths: ["/"]
      identities: ["tag:backup"]
      permission: full-access
```

### Tailscale: User-basierter Zugriff

Bestimmte Tailscale-User bekommen Zugriff auf dedizierte Repos.

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

### Tailscale: Shared Repo mit FQDN

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

### Plain: IP-basiert mit Reverse-Proxy

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

### Plain: Hostname-basiert mit Custom DNS

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

Clients dürfen schreiben aber nicht löschen — ideal für Ransomware-Schutz:

```yaml
acl:
  default_role: deny
  rules:
    - paths: ["/"]
      identities: ["*"]
      permission: append-only
```

> **Hinweis:** Für globalen Append-Only-Modus ohne Identity-basierte Differenzierung kann alternativ `append_only: true` auf Top-Level gesetzt werden. Die ACL bietet feinere Kontrolle pro Identity und Pfad.

## Middleware-Reihenfolge

Die Request-Pipeline sieht so aus:

```
Request → RepoPrefix → Recover → RequestID → Logger → Identity → ACL → Handler
```

1. **RepoPrefix** extrahiert den Repo-Pfad-Prefix und schreibt die URL um
2. **Identity** löst die Client-IP per WhoIs (Tailscale) oder rDNS (Plain) auf und speichert die Identities im Context
3. **ACL** liest die Identities aus dem Context und prüft gegen die Regeln

## Architektur

### Relevante Dateien

| Datei | Beschreibung |
|---|---|
| `internal/config/config.go` | `ACLConfig`-Struct, Validation |
| `internal/acl/acl.go` | ACL-Engine: Regelauswertung, Pfad- und Identity-Matching |
| `internal/acl/acl_test.go` | Unit-Tests für die ACL-Engine |
| `internal/middleware/identity.go` | rDNS-Resolver, Cache, Identity-Middleware |
| `internal/middleware/acl.go` | ACL-Middleware: liest Identities, erzwingt Permissions |
| `cmd/serve.go` | Wiring: `buildACLEngine`, `buildIPExtractor`, `buildIdentityMiddleware` |
