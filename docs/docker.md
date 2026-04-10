# Docker

## Image bauen

```bash
docker build -t ts-restic-server .
```

## Betrieb mit Docker Compose

Lege ein Verzeichnis an, z.B. `ts-restic-server/`, mit folgender Struktur:

```text
ts-restic-server/
├── compose.yaml
└── config.yaml
```

### config.yaml

```yaml
listen: ":8880"
listen_mode: plain
append_only: false
log_level: info

storage:
  backend: filesystem
  path: /data
```

### compose.yaml

```yaml
services:
  ts-restic-server:
    image: ts-restic-server
    # build: /pfad/zum/repo   # alternativ lokal bauen
    ports:
      - "8880:8880"
    volumes:
      - ./config.yaml:/etc/ts-restic-server/config.yaml:ro
      - data:/data
    command: ["serve", "--config", "/etc/ts-restic-server/config.yaml"]
    restart: unless-stopped

volumes:
  data:
```

### Starten

```bash
docker compose up -d
```

### Testen

```bash
RESTIC_PASSWORD=test restic -r rest:http://localhost:8880/test init
RESTIC_PASSWORD=test restic -r rest:http://localhost:8880/test backup .
RESTIC_PASSWORD=test restic -r rest:http://localhost:8880/test snapshots
```

## Andere Backends

Passe `config.yaml` entsprechend an. Beim Memory-Backend entfällt das `data`-Volume:

```yaml
storage:
  backend: memory
  max_memory_bytes: 104857600
```

Beim Rclone-Backend muss der Rclone-Server erreichbar sein:

```yaml
storage:
  backend: rclone
  rclone:
    endpoint: "http://rclone:8080"
```
