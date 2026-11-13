# pg-admin-pro

Lightweight, high-speed Database Management & Zero-Conflict Backup Studio built with **Go** + **React (TypeScript + Tailwind CSS)**.

Supports **PostgreSQL**, **MySQL / MariaDB**, and **SQLite** in a single static binary (~22 MB) with minimal RAM/CPU overhead.

## Project Layout (Local PC == VPS `/opt/pg-admin-pro`)

```text
pg-admin-pro/
├── main.go               # Go server entrypoint
├── go.mod / go.sum       # Go dependencies
├── internal/             # Auth, engines (postgres/mysql/sqlite), backup, HTTP API
├── frontend/             # React + Vite source code (PC only)
├── web/                  # Auto-generated frontend build output (vite outDir: ../web)
├── backups/              # Zero-conflict .dump / .sql snapshots
├── .env                  # Runtime configuration
└── pg-admin-pro          # Compiled single Go binary
```

## Local Development

1. Start SSH tunnel (if connecting to Edulentra VPS):
   ```bash
   cd /mnt/FA145EC5145E8513/Linux/Project/Edulentra && ./tunnel.sh start
   ```
2. Build frontend & run Go server:
   ```bash
   cd /mnt/FA145EC5145E8513/Linux/Project/pg-admin-pro
   ./build.sh
   ./pg-admin-pro
   ```
   Or during active React editing, run `go run .` in root and `npm run dev` inside `frontend/` (Vite proxies `/api` to `127.0.0.1:8080`).

## Deploy to VPS (`/opt/pg-admin-pro`)

Only copy the runtime files (`pg-admin-pro`, `web/`, `.env`) to your VPS:

```bash
./build.sh
ssh scm "sudo mkdir -p /opt/pg-admin-pro/backups"
scp pg-admin-pro .env scm:/tmp/
scp -r web scm:/tmp/
```

On the VPS:
```bash
sudo mv /tmp/pg-admin-pro /tmp/.env /opt/pg-admin-pro/
sudo rm -rf /opt/pg-admin-pro/web && sudo mv /tmp/web /opt/pg-admin-pro/
sudo chmod +x /opt/pg-admin-pro/pg-admin-pro
sudo systemctl restart pg-admin-pro
```

For production exposure over HTTPS, bind `APP_ADDR=127.0.0.1:8080` in `/opt/pg-admin-pro/.env`, set `COOKIE_SECURE=true`, and put Nginx with SSL in front.
