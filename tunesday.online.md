# tunesday.online — Implementation Plan

## 1. Overview

`tunesday.online` is the web-based successor to the `tunesday` CLI. It turns the local Tuesday ritual into an online ceremony where team members join via a shared session link and watch the provider selection together.

**Key design decisions:**
- **SQLite is the source of truth.** `tunesday.json` is used only for initial migration from the CLI and for export/import.
- **Admin has a password-protected account.** Team members join via magic links sent by email — no passwords.
- **Multiple admins per team.** Any admin can start a ceremony, invite members, and manage providers.
- **Single Go module.** The web service lives in `tunesday.online/` under the existing `tunesday` module so it can reuse `internal/core` and `internal/playlist`.
- **MVP scope:** auth, teams, providers, members, and the ceremony. Radio and quiz are Phase 2.

---

## 2. Architecture

```
┌─────────────────────────────────────────────┐
│              tunesday.online VPS                │
│                                             │
│  ┌─────────────────┐   ┌─────────────────┐  │
│  │   Caddy/nginx   │   │  Docker volume  │  │
│  │  TLS reverse    │   │   /data         │  │
│  │     proxy       │   │                 │  │
│  └────────┬────────┘   │  tunesday.db    │  │
│           │            │  exports/       │  │
│           ▼            └─────────────────┘  │
│  ┌─────────────────┐                        │
│  │  Go server      │                        │
│  │  • Chi router   │                        │
│  │  • SQLite       │                        │
│  │  • WebSocket    │                        │
│  │  • SMTP mailer  │                        │
│  │  • YouTube      │                        │
│  │    audio proxy  │                        │
│  └─────────────────┘                        │
└─────────────────────────────────────────────┘
```

### Tech Stack

| Component | Choice |
|---|---|
| Language | Go 1.26+ |
| Router | `github.com/go-chi/chi/v5` |
| Database | SQLite via `modernc.org/sqlite` (CGO-free) |
| Migrations | Raw SQL files |
| Auth | bcrypt + `gorilla/sessions` |
| Email | Standard library `net/smtp` |
| Real-time | `github.com/gorilla/websocket` |
| Frontend | Go `html/template` + vanilla JS |
| CSS | Custom terminal theme |
| Deployment | Docker + docker-compose + Caddy |

---

## 3. Data Model

### Source of Truth

SQLite is the source of truth. `tunesday.json` is only for:
1. Initial migration from the CLI tool.
2. Export for backup or portability.
3. Import for restore or migration.

### Tables

```sql
CREATE TABLE users (
    id             TEXT PRIMARY KEY,
    email          TEXT NOT NULL UNIQUE,
    password_hash  TEXT NOT NULL,
    email_verified INTEGER DEFAULT 0,
    created_at     TEXT DEFAULT (datetime('now'))
);

CREATE TABLE teams (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    admin_id    TEXT NOT NULL REFERENCES users(id),
    created_at  TEXT DEFAULT (datetime('now'))
);

CREATE TABLE providers (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    team_id     TEXT NOT NULL REFERENCES teams(id),
    name        TEXT NOT NULL,
    disabled    INTEGER DEFAULT 0,
    tune_count  INTEGER DEFAULT 0,
    UNIQUE(team_id, name)
);

CREATE TABLE tunes (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    team_id     TEXT NOT NULL REFERENCES teams(id),
    title       TEXT NOT NULL,
    link        TEXT NOT NULL,
    youtube_id  TEXT NOT NULL,
    provider_id INTEGER NOT NULL REFERENCES providers(id),
    added_at    TEXT DEFAULT (datetime('now'))
);

CREATE TABLE team_members (
    team_id       TEXT NOT NULL REFERENCES teams(id),
    user_id       TEXT NOT NULL REFERENCES users(id),
    provider_id   INTEGER NOT NULL REFERENCES providers(id),
    role          TEXT NOT NULL DEFAULT 'member',  -- 'admin' or 'member'
    magic_token   TEXT NOT NULL UNIQUE,
    PRIMARY KEY (team_id, user_id)
);

CREATE TABLE invitations (
    id             TEXT PRIMARY KEY,
    team_id        TEXT NOT NULL REFERENCES teams(id),
    email          TEXT NOT NULL,
    provider_id    INTEGER REFERENCES providers(id),
    token          TEXT NOT NULL UNIQUE,
    accepted_by    TEXT REFERENCES users(id),
    created_at     TEXT DEFAULT (datetime('now'))
);

CREATE TABLE ceremonies (
    id                TEXT PRIMARY KEY,
    team_id           TEXT NOT NULL REFERENCES teams(id),
    started_by        TEXT NOT NULL REFERENCES users(id),
    token             TEXT NOT NULL UNIQUE,
    seed              INTEGER,
    pool_json         TEXT,
    winner_provider_id INTEGER REFERENCES providers(id),
    algorithm_version TEXT DEFAULT 'bottom-half-v1',
    started_at        TEXT DEFAULT (datetime('now')),
    revealed_at       TEXT,
    completed_at      TEXT
);

CREATE TABLE ceremony_attendees (
    ceremony_id TEXT NOT NULL REFERENCES ceremonies(id),
    user_id     TEXT NOT NULL REFERENCES users(id),
    alias       TEXT NOT NULL,
    joined_at   TEXT DEFAULT (datetime('now')),
    PRIMARY KEY (ceremony_id, user_id)
);
```

### Indexes

```sql
CREATE INDEX idx_providers_team    ON providers(team_id);
CREATE INDEX idx_tunes_team        ON tunes(team_id);
CREATE INDEX idx_tunes_provider    ON tunes(provider_id);
CREATE INDEX idx_tunes_added_at    ON tunes(added_at);
CREATE INDEX idx_members_token     ON team_members(magic_token);
CREATE INDEX idx_ceremonies_token  ON ceremonies(token);
```

---

## 4. Authentication & Authorization

### Admin

- Registers with email + password.
- Verifies email via token link.
- Logs in with email + password.
- Session cookie (`tunesday_session`).
- Can create teams and manage them.

### Members

- Invited by admin via email.
- Receive a magic link: `https://tunesday.online/join/{magic_token}`.
- Clicking the link:
  - Creates a user record if missing.
  - Marks email as verified.
  - Joins the team.
  - Sets a session cookie.
- No password needed.
- Magic link is valid until revoked.
- A member can belong to multiple teams, each with its own magic link.

### Roles

| Role | Capabilities |
|---|---|
| `admin` | Start ceremonies, invite members, assign providers, add/edit providers, disable providers, export data, promote/demote admins |
| `member` | Join ceremonies, add own tunes, view team |

A team must always have at least one admin.

---

## 5. Onboarding

### Path A: Admin has a `tunesday.json`

1. Admin registers and verifies email.
2. Onboarding asks: "Do you have a tunesday.json file?" → Yes.
3. Admin uploads file.
4. System validates JSON.
5. Admin enters team name.
6. System generates slug and imports providers + tunes into SQLite.
7. Admin is redirected to team dashboard.
8. Admin assigns members to providers via invitations.

### Path B: Admin starts from scratch

1. Admin registers and verifies email.
2. Onboarding asks: "Do you have a tunesday.json file?" → No.
3. Admin enters team name.
4. System generates slug and creates empty team.
5. Admin adds provider names manually.
6. Admin invites members; invitees pick an unassigned provider name when accepting.

### Slug Generation

- Lowercase team name.
- Replace non-alphanumeric characters with hyphens.
- Collapse multiple hyphens.
- Trim leading/trailing hyphens.
- Append `-2`, `-3`, etc. on collision.

Example: `"USP Dev Team"` → `usp-dev-team`.

---

## 6. Provider Selection Ceremony

### Ceremony Rules

- Only assigned, active (non-disabled) providers are eligible.
- At least 2 eligible providers are required to start.
- Pool = bottom `ceil(n / 2)` providers by tune count.
- Ties at the cutoff count are included.
- Last submitter (provider of the tune with the latest `added_at`) is excluded from the pool when possible.
- Winner is selected uniformly at random from the remaining pool.
- Seed and pool are logged for reproducibility.

### Ceremony Flow

1. Admin clicks **"Start Ceremony"** on team dashboard.
2. Server computes eligible providers and pool.
3. Server generates seed and creates a ceremony room with token.
4. Admin shares session link: `https://tunesday.online/teams/{slug}/ceremony/{token}`.
5. Members log in via magic link and join the ceremony room.
6. Server tracks attendees via WebSocket.
7. Admin sees live attendee count.
8. Admin clicks **"Drop the Needle"**.
9. Server uses the stored seed to select winner.
10. Server broadcasts `reveal` event to all attendees.
11. Every browser plays the synchronized animation and reveals the winner.
12. Winner pastes the YouTube link for their tune.
13. Tune is saved; provider count increments; ceremony marked complete.

### WebSocket Messages

```json
{"type": "room_state", "attendees": [{"name": "Alain", "alias": "Glitchy Synthesizer"}]}
{"type": "countdown", "seconds": 3}
{"type": "reveal", "seed": 123456, "pool": ["Alain", "Lukas"], "winner": "Lukas", "duration_ms": 2500}
{"type": "complete", "winner": "Lukas"}
```

### Frontend Animation

- Retro TV lottery show aesthetic.
- Provider names rendered as spinning vinyl records.
- Drumroll via Web Audio API.
- Needle drops on the winner.
- Confetti made of `█` characters.
- Optional TTS: "The Algorithm has spoken. Today, {winner} shall provide the tunes."

---

## 7. File Structure

```
tunesday/
├── cmd/tunesday/                    # existing CLI
├── internal/
│   ├── core/                        # shared Data/Tune structs
│   ├── core/selection.go            # shared selection algorithm (moved from termui)
│   ├── playlist/                    # shared YouTube helpers
│   └── termui/                      # existing CLI TUI
├── tunesday.online/
│   ├── cmd/server/main.go
│   ├── internal/
│   │   ├── config/                  # env vars
│   │   ├── db/                      # SQLite connection + migrations
│   │   ├── migrations/              # .sql files
│   │   ├── auth/                    # register, login, verify, sessions
│   │   ├── teams/                   # team CRUD, members, invitations
│   │   ├── providers/               # provider CRUD
│   │   ├── tunes/                   # tune CRUD
│   │   ├── ceremony/                # selection, rooms, WebSocket
│   │   ├── email/                   # SMTP templates
│   │   ├── dataexport/              # JSON import/export
│   │   └── web/                     # HTTP handlers, middleware
│   ├── web/
│   │   ├── templates/
│   │   │   ├── base.html
│   │   │   ├── landing.html
│   │   │   ├── register.html
│   │   │   ├── login.html
│   │   │   ├── verify.html
│   │   │   ├── onboarding.html
│   │   │   ├── team_dashboard.html
│   │   │   ├── team_members.html
│   │   │   ├── ceremony_host.html
│   │   │   ├── ceremony_join.html
│   │   │   └── ceremony_reveal.html
│   │   └── static/
│   │       ├── css/terminal.css
│   │       └── js/ceremony.js
│   ├── Dockerfile
│   ├── docker-compose.yml
│   ├── Makefile
│   └── Caddyfile
├── go.mod
└── Makefile
```

---

## 8. API Endpoints

### Public

```
GET  /
GET  /register
POST /register
GET  /login
POST /login
POST /logout
GET  /verify
GET  /join/{magic_token}
```

### Authenticated

```
GET    /onboarding
POST   /teams
GET    /teams/{slug}
GET    /teams/{slug}/members
POST   /teams/{slug}/invitations
DELETE /teams/{slug}/members/{user_id}
POST   /teams/{slug}/members/{user_id}/role        # promote/demote admin
POST   /teams/{slug}/members/{user_id}/provider    # assign provider
POST   /teams/{slug}/providers
PUT    /teams/{slug}/providers/{provider_id}
DELETE /teams/{slug}/providers/{provider_id}
POST   /teams/{slug}/providers/{provider_id}/disable
POST   /teams/{slug}/tunes
GET    /teams/{slug}/export
POST   /teams/{slug}/import
POST   /teams/{slug}/ceremonies
GET    /teams/{slug}/ceremonies/{token}
GET    /teams/{slug}/ceremonies/{token}/host
GET    /teams/{slug}/ceremonies/{token}/ws
POST   /teams/{slug}/ceremonies/{token}/reveal
```

---

## 9. Implementation Phases

### Phase 0 — Refactor Shared Selection Logic

- Move `selectProvider` from `internal/termui/screens.go` to `internal/core/selection.go`.
- Make it accept a generic provider/tune model.
- Add tests in `internal/core/selection_test.go`.
- Update CLI to use the shared version.

### Phase 1 — Bootstrap

- Create `tunesday.online/` directory structure.
- Add dependencies to root `go.mod`.
- Create `cmd/server/main.go`.
- Create `internal/config` for env vars.
- Create `internal/db` with SQLite connection and migration runner.
- Create initial migration files.
- Add `tunesday.online/Dockerfile`, `docker-compose.yml`, `Makefile`, `Caddyfile`.

### Phase 2 — Admin Auth

- Registration page and handler.
- Email verification token generation and verification page.
- Login/logout handlers.
- bcrypt password hashing.
- Session cookie middleware.
- SMTP email service for verification emails.

### Phase 3 — Team Creation & Onboarding

- Onboarding page: "Do you have a tunesday.json?"
- Team creation without JSON.
- Team creation with JSON upload and import.
- Slug generation.
- Team dashboard skeleton.

### Phase 4 — Providers

- Add provider manually.
- Edit/disable/delete provider.
- Provider list page.
- Recalculate tune counts after import.

### Phase 5 — Member Invitations

- Invite member by email (admin assigns provider if JSON exists).
- Invitation email template with magic link.
- Join page: accept invitation, create user if missing, set cookie.
- Member list page.
- Assign/reassign provider.
- Promote/demote admin.
- Revoke member / invalidate magic token.

### Phase 6 — Tunes

- Add tune via web UI.
- List tunes.
- Increment provider tune count.
- Export team data to JSON.
- Import team data from JSON (merge or replace).

### Phase 7 — Ceremony Backend

- Create ceremony endpoint.
- Compute pool using shared selection logic.
- WebSocket room management.
- Reveal endpoint: run selection, log winner, broadcast.
- Ceremony audit logging.

### Phase 8 — Ceremony Frontend

- Host page.
- Join page.
- Vinyl roulette animation.
- WebSocket client.
- Winner reveal + add tune form.

### Phase 9 — Docker & Deployment

- Finalize Dockerfile and docker-compose.
- Add Caddy reverse proxy with auto-TLS.
- Document VPS setup, DNS, env vars.

### Phase 10 — Tests & Polish

- Unit tests for selection, providers, invitations.
- Integration tests for auth and ceremony flow.
- Mobile-friendly CSS.
- Error handling and logging.

---

## 10. Configuration

Env vars consumed by `tunesday.online/internal/config`:

| Variable | Default | Description |
|---|---|---|
| `TUNESDAY_ONLINE_DATA_DIR` | `/data` | Data directory |
| `TUNESDAY_ONLINE_LISTEN_ADDR` | `:8080` | Server bind address |
| `TUNESDAY_ONLINE_BASE_URL` | required | `https://tunesday.online` |
| `TUNESDAY_ONLINE_SESSION_SECRET` | required | Cookie signing secret |
| `TUNESDAY_ONLINE_SESSION_SECURE` | `true` | Secure cookie flag |
| `TUNESDAY_ONLINE_BCRYPT_COST` | `10` | bcrypt cost |
| `TUNESDAY_ONLINE_SMTP_HOST` | required | SMTP server |
| `TUNESDAY_ONLINE_SMTP_PORT` | `587` | SMTP port |
| `TUNESDAY_ONLINE_SMTP_USER` | required | SMTP username |
| `TUNESDAY_ONLINE_SMTP_PASS` | required | SMTP password |
| `TUNESDAY_ONLINE_SMTP_FROM` | `noreply@tunesday.online` | From address |
| `TUNESDAY_ONLINE_SQLITE_PATH` | `{DATA_DIR}/tunesday.db` | SQLite file path |

---

## 11. Docker & Deployment

### Dockerfile

```dockerfile
FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 go build -o server ./tunesday.online/cmd/server

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /build/server .
COPY --from=builder /build/tunesday.online/web ./web
EXPOSE 8080
VOLUME ["/data"]
ENV TUNESDAY_ONLINE_DATA_DIR=/data
CMD ["./server"]
```

Build from repository root:

```sh
docker build -f tunesday.online/Dockerfile -t tunesday.online:latest .
```

### docker-compose.yml

```yaml
services:
  tunesday:
    build:
      context: ..
      dockerfile: tunesday.online/Dockerfile
    ports:
      - "127.0.0.1:8080:8080"
    volumes:
      - ./data:/data
    environment:
      - TUNESDAY_ONLINE_DATA_DIR=/data
      - TUNESDAY_ONLINE_BASE_URL=https://tunesday.online
      - TUNESDAY_ONLINE_SESSION_SECRET=${SESSION_SECRET}
      - TUNESDAY_ONLINE_SMTP_HOST=${SMTP_HOST}
      - TUNESDAY_ONLINE_SMTP_PORT=${SMTP_PORT}
      - TUNESDAY_ONLINE_SMTP_USER=${SMTP_USER}
      - TUNESDAY_ONLINE_SMTP_PASS=${SMTP_PASS}
      - TUNESDAY_ONLINE_SMTP_FROM=${SMTP_FROM:-noreply@tunesday.online}
    restart: unless-stopped

  caddy:
    image: caddy:2
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./tunesday.online/Caddyfile:/etc/caddy/Caddyfile
      - caddy_data:/data
      - caddy_config:/config
    restart: unless-stopped

volumes:
  caddy_data:
  caddy_config:
```

### Caddyfile

```
tunesday.online {
    reverse_proxy tunesday:8080
}
```

### VPS Setup Checklist

1. Point `tunesday.online` A/AAAA records to VPS.
2. Install Docker and docker-compose.
3. Clone repo.
4. `cp tunesday.online/.env.example tunesday.online/.env`
5. Fill in env vars.
6. `cd tunesday.online && docker compose up -d`
7. Caddy auto-provisions TLS.

---

## 12. Open Questions / Future Work

### Phase 2 Features

- **Radio mode:** web player with queue and stats.
- **Quiz:** server-integrated "Guess the Provider" with leaderboard.
- **Scheduled ceremonies:** recurring Tuesday ceremonies.
- **Tune of the week:** auto-generated from play stats.
- **Equalizer visualization:** Web Audio API canvas bars.
- **CLI sync:** `--sync` flag for CLI to push/pull from tunesday.online API.

### Decisions to Confirm

- Should exported JSON include `tunesday.json` fields exactly (including recalculated counts), or a richer format?
- Should import merge or replace existing team data?
- Should we use UUIDs or ULIDs for primary keys?

---

## 13. Success Criteria

- Admin can register, verify email, and create a team.
- Admin can invite members by email.
- Members can join via magic link without a password.
- Admin can start a ceremony and share the session link.
- Members can join the ceremony and see the synchronized winner reveal.
- Winner can add their tune.
- Team data can be exported/imported as JSON.
- Deployment works via Docker on a VPS at `https://tunesday.online`.
