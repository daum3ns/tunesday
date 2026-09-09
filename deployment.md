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

# Open firewall ports for Caddy (Let's Encrypt ACME challenges + HTTPS)
ufw allow 80/tcp
ufw allow 443/tcp
ufw reload

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
| `TUNESDAY_ONLINE_IMAGE_TAG` | `v1.0.3` (bump to the desired release tag) |

Generate a session secret:

```bash
openssl rand -hex 32
```

## Step 4: Deploy

```bash
cd /opt/tunesday/tunesday.online
git pull                       # fetch latest compose/Caddyfile (config only)
docker compose pull            # pull the published image from GHCR
docker compose up -d
```

This will:
1. Pull the **released** image `ghcr.io/daum3ns/tunesday:<tag>` from GHCR (public registry — no `docker login` needed) with the Go binary, yt-dlp, and the backup script preinstalled
2. Start the Go server on `127.0.0.1:8080`
3. Start Caddy on ports 80/443
4. Caddy auto-provisions a Let's Encrypt TLS cert

The image tag it pulls is `TUNESDAY_ONLINE_IMAGE_TAG` from `.env`
(default `v1.0.3` if unset). The running version is always exactly that tag —
you can see it in the page footer and via the `VERSION` build flag baked at release time.

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
| View logs | `docker compose logs -f tunesday` |
| Restart | `docker compose restart` |
| Update to a new release | `git pull` → set `TUNESDAY_ONLINE_IMAGE_TAG=vX.Y.Z` in `.env` → `docker compose pull && docker compose up -d` |
| Roll back | set `TUNESDAY_ONLINE_IMAGE_TAG` back to the previous version → `docker compose pull && docker compose up -d` |
| Update yt-dlp | automatically included in the next image release; in-container `pip install -U yt-dlp` is ephemeral and lost on recreate |
| Backup DB | automatic daily via `backup` sidecar; manual: `docker compose exec backup /app/scripts/backup.sh`; view: `ls data/backups/` |

### Deploying an update (run through)

1. Stash any local edits: `git stash` (or commit on a branch)
2. `git pull`
3. Edit `.env`: `TUNESDAY_ONLINE_IMAGE_TAG=<new tag>`
4. `docker compose pull && docker compose up -d`
5. Verify in the browser that the footer shows the new tag

> Rollback is just step 3 with the previous tag. No rebuilds ever happen on the VPS.

### First cutover from a build-based install

If the server previously ran locally-built images:

```bash
docker compose up -d --no-build   # recreate from the pulled image
docker image prune -f             # drop old local layers
```

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
