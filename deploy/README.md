# Deploying WheelMUD

Two supported deployment paths: Docker (recommended for self-hosters) and bare-metal systemd (recommended for the production server).

Both consume the artifacts published by the release pipeline (`.github/workflows/release.yml`) on tag push: tarball binaries via `goreleaser` and a multi-arch container image at `ghcr.io/jasrags/wheelmud`.

## Docker

```bash
docker run -d \
  --name wheelmud \
  -p 2323:2323 \
  -p 127.0.0.1:9090:9090 \
  -v wheelmud-data:/var/lib/wheelmud \
  -v wheelmud-backups:/var/backups/wheelmud \
  ghcr.io/jasrags/wheelmud:latest
```

Or use `docker compose up -d` from the repo root — the compose file in the repo wires the volumes, healthcheck, and metrics-loopback binding for you.

The container ships with these defaults (override via env or by mounting a `config.yaml` into `/etc/wheelmud`):

| Setting | Default | Env override |
| --- | --- | --- |
| Telnet bind | `:2323` | `LISTEN_ADDR` |
| Metrics bind | `0.0.0.0:9090` (container) | `METRICS_ADDR` |
| DB | `/var/lib/wheelmud/wheelmud.db` | `DB_DSN` |
| Backups | `/var/backups/wheelmud` | `BACKUP_DIR` |
| World tree | `/var/lib/wheelmud/data/world` | `WORLD_DIR` |
| Log level | `info` | `LOG_LEVEL` |

The HEALTHCHECK probes `/healthz`; `docker inspect <container>` reports `Status: healthy` once the server is ready and the DB ping passes.

## Bare-metal (systemd)

1. Create the service user and directories:

   ```bash
   sudo useradd --system --no-create-home --shell /sbin/nologin wheelmud
   sudo mkdir -p /var/lib/wheelmud /etc/wheelmud /var/backups/wheelmud /var/lib/wheelmud/data
   sudo chown -R wheelmud:wheelmud /var/lib/wheelmud /var/backups/wheelmud
   ```

2. Drop the binary and world tree from the release tarball:

   ```bash
   tar -xzf wheelmud_<version>_linux_amd64.tar.gz
   sudo install -m 0755 wheelmud-server /usr/local/bin/wheelmud-server
   sudo cp -r data/world /var/lib/wheelmud/data/
   sudo chown -R wheelmud:wheelmud /var/lib/wheelmud/data
   sudo install -m 0644 config.example.yaml /etc/wheelmud/config.yaml
   ```

   Edit `/etc/wheelmud/config.yaml` — at minimum set `db.dsn` to `/var/lib/wheelmud/wheelmud.db` and `db.backup_dir` to `/var/backups/wheelmud`.

3. Install the systemd unit:

   ```bash
   sudo install -m 0644 deploy/systemd/wheelmud.service /etc/systemd/system/wheelmud.service
   sudo systemctl daemon-reload
   sudo systemctl enable --now wheelmud
   ```

4. Verify:

   ```bash
   systemctl status wheelmud
   journalctl -u wheelmud -f
   curl http://127.0.0.1:9090/healthz   # 200 ok
   ```

The unit applies a conservative hardening profile (`NoNewPrivileges`, `ProtectSystem=strict`, read-write paths limited to `/var/lib/wheelmud` and `/var/backups/wheelmud`).

## Observability

- **Metrics:** Scrape `http://<host>:9090/metrics` from Prometheus. Series live under the `wheelmud_*` prefix; the Go and Process collectors are also registered.
- **Healthz:** `http://<host>:9090/healthz` returns 200 ok when the server is ready and the DB pings, 503 otherwise. Suitable for liveness probes and Docker HEALTHCHECK.
- **pprof:** `http://<host>:9090/debug/pprof/` exposes the stdlib pprof endpoints. Loopback-only by default; keep it that way in production.
- **Backups:** `db.backup_dir` accumulates `wheelmud-YYYYMMDD-HHMMSS.db` snapshots on the cadence configured by `db.backup_interval_hours`. Retention prunes oldest first.
- **Audit:** Set `audit.commands_enabled: true` to record one `character_audit` row per dispatched player command.

## Upgrading

Each release ships a fresh DB schema. The server runs forward-only migrations at boot; downgrades are not supported. Always take a backup snapshot before upgrading: copy the most recent file under `db.backup_dir`, or run `VACUUM INTO 'pre-upgrade.db'` against a paused instance.

The migration log is in `internal/db/migrations/`. Inspect new files between your current and target tag before upgrading large worlds.
