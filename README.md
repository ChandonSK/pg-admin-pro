# pg-admin-pro

A lightweight, high-speed **Database Management & Zero-Conflict Backup Studio** built with **Go** + **React (TypeScript + Tailwind CSS)**.

Created by **SKP ([ChandonSK](https://github.com/ChandonSK/pg-admin-pro))** to make PostgreSQL administration (along with MySQL / MariaDB and SQLite) simpler, faster, and easier to self-host—shipping as a single static binary (`~16 MB`) with minimal RAM and CPU overhead.

---

## Why `pg-admin-pro`?

Traditional database web tools can be heavy to deploy and slow to navigate for day-to-day server administration. `pg-admin-pro` is designed to provide an all-in-one, responsive studio that runs identically on your local machine or directly on your Linux VPS:

- **Single binary + static `web/` folder** — no Python runtime, Docker stack, or external daemon required.
- **Zero-locking MVCC backups** via `pg_dump -Fc`, parallel `pg_restore -j`, `psql`, `mysqldump`, and SQLite `VACUUM INTO`.
- **Full DBA & developer workflow** — schema DDL editing, inline data editing, FK jump navigation, multi-tab SQL IDE, live activity monitor, and S3/R2 off-site backup sync.

---

## Key Features

- **Multi-Engine Support:** PostgreSQL, MySQL / MariaDB, and SQLite.
- **Table & Schema DDL Editor:** Create/drop tables, add/alter/drop columns, create/drop indexes, manage Foreign Key / Unique / Check constraints, truncate tables, run `VACUUM ANALYZE`, and refresh materialized views.
- **Data Grid UX:** Paginated browsing, filtering & sorting, double-click inline cell editing, Foreign Key click-to-jump navigation, automatic `ENUM` dropdowns, CSV/JSON/SQL export, and CSV/JSON import with column mapping.
- **SQL IDE & Execution Plans:** Multi-tab query editor (`localStorage`), schema-aware autocomplete, SQL formatter, saved snippets + DBA templates, and `EXPLAIN` / `EXPLAIN ANALYZE` plan viewer (safely rolled back inside a transaction).
- **Live Activity Monitor & Objects Browser:** Inspect `pg_stat_activity`, locks, cache hit ratio, table bloat/scans, unused indexes, cancel/terminate queries, and browse Functions, Triggers, Sequences, Custom Types/ENUMs, and Extensions.
- **Saved Server Profiles:** Store multiple database connections encrypted with **AES-256-GCM** and switch servers directly from the navbar without logging out.
- **Backup Automation & S3/R2 Sync:** Upload/download `.dump` & `.sql` backups, restore with `pg_restore`/`psql`/`mysql`, schedule daily automated backups with retention pruning, and sync snapshots to AWS S3, Cloudflare R2, or MinIO.

---

## Project Structure

```text
pg-admin-pro/
├── main.go                      # Go server entrypoint
├── go.mod / go.sum              # Go dependencies
├── internal/                    # Auth, engines (postgres/mysql/sqlite), backup, HTTP API
├── frontend/                    # React + TypeScript + Vite source code
├── web/                         # Generated frontend build output (built by ./build.sh)
├── backups/                     # Stored backup snapshots (ignored by git)
├── data/                        # Encrypted profiles, snippets & backup settings (ignored by git)
├── .env.example                 # Example runtime configuration
├── pg-admin-pro.service.example # Example systemd unit file for Linux servers
└── build.sh                     # Builds frontend -> web/ and compiles static Go binary
```

---

## Quick Start (Local Usage)

### Prerequisites
- **Go 1.22+** and **Node.js 18+** (for building from source)
- Optional CLI tools on the host machine for backup/restore operations:
  - PostgreSQL: `pg_dump`, `pg_restore`, `psql` (`postgresql-client`)
  - MySQL / MariaDB: `mysqldump`, `mysql`

### 1. Clone & Configure

```bash
git clone https://github.com/ChandonSK/pg-admin-pro.git
cd pg-admin-pro
cp .env.example .env
```

Edit `.env` if you want to customize `APP_ADDR`, `DATA_DIR`, `BACKUP_DIR`, or login form defaults.

### 2. Build & Run

```bash
chmod +x build.sh
./build.sh
./pg-admin-pro
```

Open **`http://localhost:8080`** in your browser and sign in with your database credentials (PostgreSQL, MySQL, or SQLite file path).

### Development Mode (Hot Reload)

Run the Go backend and Vite dev server in two terminals:

```bash
# Terminal 1 (Go API on :8080)
go run .

# Terminal 2 (React dev server with /api proxy)
cd frontend && npm install && npm run dev
```

### Connecting to a Remote Database from Your PC

If your database only listens on `127.0.0.1` on a remote server, forward the port over SSH first:

```bash
ssh -N -L 5432:127.0.0.1:5432 user@your-server-ip
```

Then connect to `127.0.0.1:5432` in the `pg-admin-pro` login screen.

---

## Production Deployment (Linux VPS)

For running `pg_dump` / `pg_restore` at full local speed without network overhead, deploy `pg-admin-pro` directly on your database server (for example under `/opt/pg-admin-pro`).

### 1. Build & Upload Runtime Files

Only `pg-admin-pro`, `web/`, and `.env` are needed on the server:

```bash
./build.sh
ssh user@your-server "sudo mkdir -p /opt/pg-admin-pro/backups /opt/pg-admin-pro/data"
scp pg-admin-pro .env user@your-server:/tmp/
scp -r web user@your-server:/tmp/
```

### 2. Install & Start Systemd Service

On the server:

```bash
sudo mv /tmp/pg-admin-pro /tmp/.env /opt/pg-admin-pro/
sudo rm -rf /opt/pg-admin-pro/web && sudo mv /tmp/web /opt/pg-admin-pro/
sudo chmod +x /opt/pg-admin-pro/pg-admin-pro
sudo chown -R www-data:www-data /opt/pg-admin-pro
```

Copy `pg-admin-pro.service.example` to `/etc/systemd/system/pg-admin-pro.service` and enable it:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now pg-admin-pro
```

> **Security Tip:** In production behind Nginx/Caddy HTTPS, set `APP_ADDR=127.0.0.1:8080` and `COOKIE_SECURE=true` in `/opt/pg-admin-pro/.env`.
