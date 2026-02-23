# Testing

## Voraussetzungen

| Voraussetzung | Benötigt für |
|---|---|
| Go (siehe `go.mod`) | Alle Tests |
| `restic` Binary | Integrationstests |
| Docker | S3-Backend-Test (MinIO-Container) |

## Tests ausführen

```bash
# Alle Tests (Unit + Integration)
go test ./...

# Nur Unit-Tests / schnelle Tests (Integrationstests werden übersprungen)
go test -short ./...

# Nur Integrationstests
go test -v ./tests/integration/

# Einzelnes Backend testen
go test -v -run TestMemoryBackend      ./tests/integration/
go test -v -run TestFilesystemBackend  ./tests/integration/
go test -v -run TestWebDAVBackend      ./tests/integration/
go test -v -run TestRcloneBackend      ./tests/integration/
go test -v -run TestS3Backend          ./tests/integration/
```

## Integrationstests

Die Integrationstests prüfen den vollständigen Restic-Lifecycle gegen jedes Storage-Backend:

1. Testdaten generieren (deterministische Zufallsdaten, seed-basiert)
2. `restic init`
3. `restic backup` (Snapshot 1)
4. Delta-Dateien hinzufügen, `restic backup` (Snapshot 2)
5. Snapshot-Liste prüfen (erwartet: 2)
6. `restic restore` Snapshot 1 + SHA-256-Hash-Vergleich
7. `restic restore` Snapshot 2 + SHA-256-Hash-Vergleich
8. `restic forget --prune` Snapshot 1
9. Snapshot-Liste prüfen (erwartet: 1)

### Testdaten

**Dataset A (Mixed-Size, ~100 MB):**
- 50 × 1 MB Dateien
- 100 × 100 KB Dateien
- 200 × 10 KB Dateien
- 10 × 512 KB Delta-Dateien (für Snapshot 2)

**Dataset B (Single Large File, 100 MB):**
- 1 × 100 MB Datei (Edge Case für Pack-Handling)

Alle Daten sind schlecht komprimierbar (Zufallsdaten) und deterministisch (fester Seed).

### Backend-Testmatrix

| Backend | Externe Abhängigkeit | Infrastruktur |
|---|---|---|
| Memory | Keine | In-Process |
| Filesystem | Keine | `t.TempDir()` |
| WebDAV | Keine | In-Process Go WebDAV-Server |
| Rclone | Keine | Zweite ts-restic-server-Instanz mit Memory-Backend |
| S3 | Docker | MinIO via testcontainers-go |

### Skip-Verhalten

Tests werden automatisch übersprungen wenn Voraussetzungen fehlen:

| Bedingung | Auswirkung |
|---|---|
| `-short` Flag | Alle Integrationstests übersprungen |
| `restic` Binary fehlt | Alle Integrationstests übersprungen |
| Docker nicht verfügbar | Nur S3-Test übersprungen |

## CI-Pipeline

Die GitHub Actions Pipeline besteht aus zwei Stufen:

1. **Unit & Vet** — `go vet` + `go test -short` (schnell, kein restic/Docker nötig)
2. **Integration** — Ein Job pro Backend (parallel, Matrix-Build), läuft nur wenn Unit-Tests bestehen

Die S3-Tests laufen mit einem ephemeren MinIO-Container via testcontainers-go. Docker ist auf den GitHub Actions Runnern vorinstalliert.
