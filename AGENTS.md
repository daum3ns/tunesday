# Tunesday.online — Agent Guide

## Quick start
```sh
make build-server  # go build -o build/tunesday.online ./tunesday.online/cmd/server
make test          # builds first, then go clean -testcache && go test -v ./...
make clean         # rm -f build/*
```

Single-test shortcut: `go test -v -run TestName ./tunesday.online/internal/package`

## Key facts
- **Go 1.26+** required (see `go.mod`).
- **Data file**: `tunesday.json` in CWD (gitignored). Used for web import only.
- **YouTube title fetch** uses `github.com/kkdai/youtube/v2` — no API key needed. Accepts `watch`, `youtu.be`, `/shorts/`, `music.youtube.com` URLs.
- **Radio stream** uses `yt-dlp` (`TUNESDAY_ONLINE_YTDLP_PATH` env override). Browser plays audio directly from Google CDN (URL-return JSON, no server proxy).
- **Auth**: admin email+password; members join via magic links (`/join/{token}`), no passwords.
- **SQLite** via `modernc.org/sqlite` (CGO-free). In-memory DB needs `SetMaxOpenConns(1)`.
- **Master admin**: set via `TUNESDAY_MASTER_ADMIN_EMAIL` env var. Bypasses all team membership/admin checks.
- **No linter/formatter/typecheck config.** Use standard `go fmt`, `go vet`.

## Architecture
```
cmd/server/         — main(), config, Docker entrypoint
internal/
  auth/             — session store (cookie), middleware, magic links
  web/              — HTTP handlers, templates, static assets, WebSocket endpoints
  store/            — SQLite repositories (users, teams, tunes, ceremonies, quiz, play_stats)
  core/             — Data + Tune structs, provider selection algorithm
  playlist/         — YouTube URL normalization + title fetching
  stream/           — yt-dlp based audio stream resolver (cached URLs)
  dataimport/       — tunesday.json import + team creation
  radio/            — per-user radio presence manager
  live/             — ceremony WebSocket rooms
  db/               — SQLite wrapper + embedded migrations
  email/            — SMTP service
  config/           — env-based configuration
```

## Testing quirks
- `make test` always builds before testing and clears test cache.
- Tests use `t.TempDir()` for temp files — no fixtures needed.
- **All playback/stream tests require `yt-dlp` installed** — `NewYTDLP()` calls `Available()`.
- Packages `auth`, `config`, `email`, `live` have **no test files**.
- Web E2E tests spin up a real HTTP server on `127.0.0.1` with an in-memory SQLite DB.

## Known gotchas
- Ceremony pool = **connected eligible attendees minus last submitter**, winner picked uniformly at random (`revealPool` in `ceremony_handlers.go`). The old bottom-half `SelectProvider` in `internal/core` was removed.
- `requireMember` returns a synthetic admin `TeamMember` for master admin — downstream code must handle this.
- Ceremony countdown is hardcoded to 5000ms (`ceremony_handlers.go`).
- The `webhook.md` planned Teams notification feature was never implemented and has been removed.
