# Tailscale Setup

In `listen_mode: tailscale` the server joins your tailnet via [tsnet](https://pkg.go.dev/tailscale.com/tsnet) and serves HTTPS on port 443 with an automatic certificate.

The Tailscale auth key is only needed once: on first start, when `tailscale.state_dir` is empty, it registers the node. Afterwards the node identity lives in the state dir and the key is ignored. The `init` command uses this to keep the key off the host.

## Recommended flow

```yaml
listen_mode: tailscale
tailscale:
  hostname: restic-server
  state_dir: /var/lib/ts-restic-server/ts-state
```

```bash
# 1. Register the node once. The key only exists for this call.
echo "$TS_AUTHKEY" | ts-restic-server init --auth-key-stdin

# 2. Run the server without any auth key.
ts-restic-server serve
```

Use an absolute `state_dir`. A relative path resolves against the working directory, so `init` and `serve` could end up using different directories.

Run `init` as the same user as `serve` (e.g. `sudo -u ts-restic-server ts-restic-server init ...`). Otherwise the state dir belongs to the wrong user and `serve` cannot read it. `init` prints a warning when it runs as root.

## `init`

```text
ts-restic-server init [--config FILE] [--auth-key KEY | --auth-key-stdin] [--timeout 2m] [--force] [--verbose]
```

`init`:

1. Starts tsnet with the configured `hostname` and `state_dir` and the auth key.
2. Waits until the node is up (logged in, IP assigned) and fetches its TLS certificate, so the state dir is complete.
3. Shuts down cleanly and exits 0.

If the state dir already holds a valid node identity, tsnet ignores the key and `init` reports `already initialized` and exits 0. This makes it safe to run repeatedly, e.g. from Ansible.

### Auth key sources

Checked in this order; the first one found wins:

| Source | Notes |
|--------|-------|
| `--auth-key KEY` | Visible in the process list. Prefer stdin or the environment. |
| `--auth-key-stdin` | Reads the first line of stdin. Cannot be combined with `--auth-key`. |
| `TS_AUTHKEY`, `TS_AUTH_KEY`, `RESTIC_TAILSCALE_AUTH_KEY` | Environment, in this order. |
| `tailscale.auth_key` in the config file | Still supported, prints a warning. A `${VAR}` placeholder counts as environment. |

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--timeout` | `2m` | How long to wait for the node to come up. |
| `--force` | `false` | Re-register with the given key even if node state exists (e.g. the node was removed from the tailnet). Needs a key. |
| `--verbose` | `false` | Print tsnet logs. |

`init` only reads the `tailscale` section. Unset `${VAR}` placeholders elsewhere in the config (e.g. storage secrets) do not stop it.

### Errors

| Message | Cause |
|---------|-------|
| `init is only needed for listen_mode: tailscale` | The config uses plain mode. |
| `no auth key given and no Tailscale state in <dir>` | First registration without a key. |
| `node state in <dir> is no longer valid` | The node wants a new login (removed or expired) and no key was given. |
| `the auth key was not used to log in ...; re-run with --force` | State exists, but the node is not logged in; tsnet only uses the key when forced. |
| `node did not come up within <timeout>` | Network, invalid key, or the node waits for approval in the admin console. |
| `tailnet provides no certificate domain` | Enable MagicDNS and HTTPS certificates in the Tailscale admin console. |

## `serve` behavior

| Node state | Auth key (environment or config) | `interactive_login` | Result |
|------------|----------------------------------|---------------------|--------|
| present | any | any | starts; a key in the config file triggers a "no longer needed" warning |
| missing | yes | any | registers with the key as before, with a warning if it comes from the config file |
| missing | no | `true` | logs a browser login URL and waits (previous behavior) |
| missing | no | `false` (default) | exits with `no Tailscale node state in <dir>: run ts-restic-server init first` |

If the node later asks for a browser login (for example after it was removed from the tailnet) and neither a key nor `interactive_login` is set, `serve` shuts down with an error instead of waiting forever.

Enable interactive login with `--tailscale-interactive-login` or in the config:

```yaml
tailscale:
  interactive_login: true
```

## Migrating from `auth_key` in the config

1. Make sure `state_dir` already contains the node state (the server has run at least once).
2. Remove `auth_key` from the config (and `RESTIC_TAILSCALE_AUTH_KEY` from the environment).
3. Restart `serve`. It keeps using the existing node identity.

For new hosts, run `init` with a short-lived, single-use key before the first start.
