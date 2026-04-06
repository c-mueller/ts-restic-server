# SMB/CIFS Storage Backend

The SMB backend stores restic repositories on a remote SMB/CIFS share (Samba, Windows file shares, NAS devices). It uses a pure-Go SMB2/3 client library and does **not** require mounting the share via the OS kernel.

## Configuration

```yaml
storage:
  backend: smb
  smb:
    server: "nas.local"          # SMB server hostname or IP
    share: "backups"             # share name
    username: "restic"           # SMB username
    password: "${SMB_PASSWORD}"  # supports env var substitution
    domain: "WORKGROUP"          # SMB domain (default: WORKGROUP)
    port: 445                    # SMB port (default: 445)
    base_path: "restic-repos"   # optional subdirectory within the share
```

### Required fields

| Field    | Description                         |
|----------|-------------------------------------|
| `server` | Hostname or IP of the SMB server    |
| `share`  | Name of the SMB share to connect to |

### Optional fields

| Field       | Default     | Description                                    |
|-------------|-------------|------------------------------------------------|
| `username`  | (empty)     | SMB authentication username                    |
| `password`  | (empty)     | SMB authentication password                    |
| `domain`    | `WORKGROUP` | SMB/NTLM domain                               |
| `port`      | `445`       | TCP port for SMB connection                    |
| `base_path` | (empty)     | Subdirectory within the share for all repos    |

## How it works

- Connects to the SMB share using the SMB2/3 protocol via `github.com/hirochachacha/go-smb2`
- Each repository is stored as a directory tree under `base_path/<repo-prefix>/`
- Standard restic directory layout: `config`, `data/`, `keys/`, `locks/`, `snapshots/`, `index/`
- Atomic writes: files are written to a temporary name and renamed on completion
- Automatic reconnection if the SMB session is lost
- Thread-safe: all operations are serialized via mutex

## NAS compatibility

Tested and designed to work with:

- Samba (Linux/macOS)
- Synology DSM
- QNAP QTS
- Windows file shares
- Any SMB2/3 compatible server

## CLI usage

```bash
ts-restic-server serve --storage-backend smb --config config.yaml
```

The `server` and `share` fields must be set in the config file; there are no dedicated CLI flags for SMB-specific options.

## Security notes

- Credentials can use `${ENV_VAR}` substitution to avoid storing passwords in config files
- The connection uses NTLM authentication over SMB2/3
- Consider using `--env-lenient` only in development; in production, ensure all env vars are set
