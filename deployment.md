# tunesday.online — Deployment Plan

## Architecture

```
Internet → Caddy (TLS, auto-cert) → Go server (:8080)
                                          |
                                     SQLite DB (/data/tunesday.db)
                                     yt-dlp (radio streams + titles)
                                     SMTP (external)
```

Everything runs in Docker. No Go, Node, or nginx required on the server.

## Minimum Server Requirements

| Resource | Minimum | Recommended |
|----------|---------|-------------|
| vCPU    | 1       | 2           |
| RAM     | 512 MB  | 1 GB        |
| Disk    | 10 GB SSD | 20 GB SSD |

## Step 1: Server Setup (fresh Linux)

```bash
# Update system
apt update && apt upgrade -y

# Install Docker
curl -fsSL https://get.docker.com | sh

# Install docker compose plugin
apt install -y docker-compose-plugin

# Verify
docker --version
docker compose version
```

## Step 2: DNS

Point your domain to the VPS IP **before** starting Caddy (it needs DNS to issue TLS certs):

```
A record:    tunesday.online → YOUR_VPS_IP
AAAA record: tunesday.online → YOUR_VPS_IP  (if IPv6)
```

## Step 3: Clone & Configure

```bash
cd /opt
git clone git@github.com:daum3ns/tunesday.git
cd tunesday/tunesday.online

# Create .env from example
cp .env.example .env
```

Edit `.env` with your values:

```bash
nano .env
```

| Variable | Value |
|----------|-------|
| `TUNESDAY_ONLINE_BASE_URL` | `https://tunesday.online` |
| `TUNESDAY_ONLINE_SESSION_SECRET` | `openssl rand -hex 32` |
| `TUNESDAY_ONLINE_SMTP_HOST` | `smtp.yourdomain.com` |
| `TUNESDAY_ONLINE_SMTP_PORT` | `587` (or `465`) |
| `TUNESDAY_ONLINE_SMTP_USER` | `noreply@yourdomain.com` |
| `TUNESDAY_ONLINE_SMTP_PASS` | your SMTP password |
| `TUNESDAY_ONLINE_SMTP_FROM` | `noreply@yourdomain.com` |
| `TUNESDAY_MASTER_ADMIN_EMAIL` | your email (for master admin) |

Generate a session secret:

```bash
openssl rand -hex 32
```

## Step 4: Build & Launch

```bash
cd /opt/tunesday/tunesday.online
docker compose up -d --build
```

This will:
1. Build the Go binary in a multi-stage Docker build
2. Install yt-dlp in the container
3. Start the Go server on `127.0.0.1:8080`
4. Start Caddy on ports 80/443
5. Caddy auto-provisions a Let's Encrypt TLS cert

## Step 5: Verify

```bash
# Check containers are running
docker compose ps

# Check server logs (look for master admin message)
docker compose logs tunesday

# Check Caddy logs
docker compose logs caddy

# Test health endpoint
curl -k https://tunesday.online/health
```

You should see in the tunesday logs:

```
tunesday.online: master admin set to your@email.com
tunesday.online: listening on :8080
```

## Step 6: Register & Test

1. Open `https://tunesday.online` in your browser
2. Register with the master admin email you configured
3. You should see the yellow `[ all teams ]` badge in the nav
4. Create a team, invite members, test radio/ceremony

## Ongoing Maintenance

| Task | Command |
|------|---------|
| Update yt-dlp | `docker compose exec tunesday python3 -m pip install -U yt-dlp` |
| View logs | `docker compose logs -f tunesday` |
| Restart | `docker compose restart` |
| Update code | `git pull && docker compose up -d --build` |
| Backup DB | automatic daily via `backup` sidecar; manual: `docker compose exec backup /app/scripts/backup.sh`; view: `ls data/backups/` |

## Backups & Restore

The `backup` compose service runs once daily. It uses SQLite `VACUUM INTO` to
produce a consistent single-file snapshot (safe on a live WAL-mode DB) into
`./data/backups/`, keeps the newest 7 daily backups plus 4 weekly (Sundays),
and prunes the rest. Backups live on the same disk as the DB for now.

```sh
# Manual backup
docker compose exec backup /app/scripts/backup.sh

# List backups
ls data/backups/
```

### Restore runbook

```
1. Pick a backup:          ls data/backups/
2. Stop the app:           docker compose stop tunesday
3. Safely swap the DB:
       rm -f data/tunesday.db data/tunesday.db-wal data/tunesday.db-shm
       cp data/backups/tunesday-<DATE>.db data/tunesday.db
4. Start:                  docker compose up -d tunesday
5. Verify:                 curl -k https://tunesday.online/health
   Then log in as master admin and spot-check a team, radio, and ceremony.
6. If wrong: keep the swapped-out DB aside; re-run with another backup.
```
