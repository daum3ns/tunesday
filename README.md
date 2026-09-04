# tunesday.online

We at [**@USP**](https://github.com/united-security-providers/) have a long and proud tradition: every Tuesday we wish each other
*a happy Tunesday*, appreciate how wonderful it is to have music, and — right at
the end of our daily meeting — fairly determine who gets to be the soundtrack
dealer for the week. It is, admittedly, the most democratic thing we do all day.

**tunesday.online** ([tunesday.online](https://tunesday.online)) is the online reincarnation of that ritual. Your team joins a
link, the **Tunesday Roulette** spins in glorious real-time, and somebody leaves
with the sacred duty of picking everyone's soundtrack. Along the way you get a
 **radio room**, a "guess who submitted this banger" **quiz**, and enough
**stats** to fuel a very petty, very loving rivalry.

Built with Go, SQLite, and WebSocket. Deployed with Docker + Caddy. Resistance is
futile — your Tuesday soundtrack is inevitable.

> **The tunesday CLI is deprecated.** 🪦 You can migrate your old team to
> [tunesday.online](https://tunesday.online) by import your good old `tunesday.json` file. Your tunes will
> live again.

> **Fair warning:** this project is **vibe-coded**. It runs, it's fun, and it
> absolutely has no business being as stable as it is. Proceed with a sense of
> adventure (and maybe a backup).

## Quick Start (local development)

```sh
./dev-test.sh
```

Starts MailHog (email testing) and the server at http://localhost:8080. Register, create a team, and start exploring.

## Deploy

See [deployment.md](deployment.md) for the full VPS deployment guide.

## How It Works

- **Teams**: users join via magic links. Each team has providers (members) and tunes.
- **Ceremony**: a synced roulette picks who submits a tune this week. Runs in real-time via WebSocket.
- **Radio**: per-user playback. Queue is client-side, server tracks presence and now-playing.
- **Quiz**: guess which teammate submitted a tune, based on 10-second YouTube snippets.
- **Stats**: tune of the week, play counts, ceremony attendance, quiz leaderboards.

## Provider Selection

The ceremony is a livestream ritual, so the pool is whoever **actually shows up**
(no skipping out on your sacred duty by simply not turning up):

- Pool = eligible providers whose owners are **connected to the ceremony room**
- The **most recent submitter** gets a polite (or not-so-polite) exclusion from the pool
- A winner is picked **uniformly at random** from the remaining pool

So yes: attendance matters. Elegance is preserved. Chaos is evenly distributed.

## Project Layout

```
tunesday.online/
  cmd/server/        — main(), config, Docker entrypoint
  internal/
    auth/            — session store, middleware, magic links
    web/             — HTTP handlers, templates, static assets
    store/           — SQLite repositories (users, teams, tunes, ceremonies, quiz)
    core/            — Data + Tune structs
    playlist/        — YouTube URL normalization + title fetch
    stream/          — yt-dlp based audio stream resolver
    dataimport/      — tunesday.json import + team creation
    radio/           — per-user radio presence manager
    live/            — ceremony WebSocket rooms
    db/              — SQLite wrapper + migrations
    email/           — SMTP service
    config/          — env-based configuration
```

## Tech Stack

- **Go 1.26+** — single binary, embedded static assets
- **SQLite** via `modernc.org/sqlite` (CGO-free)
- **WebSocket** via `gorilla/websocket`
- **YouTube** via `yt-dlp` (radio streams + title lookup)
- **Docker** multi-stage build + Caddy reverse proxy with auto-TLS

## License

See LICENSE. Be nice, share tunes.
