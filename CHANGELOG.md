# Changelog

All notable changes to tunesday.online are documented here.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added
- **SoundCloud & Bandcamp tune support**: the ceremony/dashboard link input
  now accepts SoundCloud and Bandcamp links in addition to YouTube. Unknown
  platforms (Spotify, Vimeo, etc.) are rejected with a clear allowlist hint;
  nothing is stored.
- A `tunes.platform` column tracks the source platform; radio streaming and
  the guard clause are driven by it rather than by `youtube_id`.
- **Manual tune add**: admins can register a tune outside any ceremony from
  the dashboard "Recent tunes" section, choosing which provider it is
  attributed to — for the fallback when the team picked a tune by hand. The
  entry can be backdated to the day it was actually picked (defaults to
  today).

### Changed
- Title fetching and stream resolution now go through a single `playlist.Normalize`
  + yt-dlp call shape (`--print "%(url)s|%(ext)s"`, no `-g`) that works for
  all three platforms. The format selector is
  `bestaudio[protocol!^=m3u8][acodec=aac]/bestaudio[protocol!^=m3u8][ext=m4a][acodec!=alac]/bestaudio[protocol!^=m3u8][acodec=mp3]/bestaudio/best`.

### Fixed
- **SoundCloud radio playback**: some SoundCloud tracks resolve to HLS
  (`…/playlist.m3u8`, `application/vnd.apple.mpegurl`), which Chromium/Firefox
  cannot decode. The stream selector now excludes m3u8 protocols (`!^=m3u8`)
  so the radio falls back to the progressive mp3 and plays everywhere.

### Known caveats
- Some SoundCloud tracks are DRM-protected and will be rejected at title-fetch
  time with the yt-dlp error surfaced in the ceremony flash.
- Bandcamp ALAC is avoided via the explicit `acodec=aac` clause in the format
  selector; without it, yt-dlp picks `falac` which is a Chromium risk.
- SoundCloud tracks offering HLS only will not play in Chromium/Firefox.

## [1.0.3] - 2026-09-15

### Added
- **Play on radio from ceremony**: after a tune is registered, live ceremony
  viewers get a "▶ play on radio" link that opens the radio with the tune loaded.
- **Play buttons in recent tunes**: the dashboard's recent tunes list has a ▶
  button per tune, navigating to the radio page with that track pre-loaded.
- **Sortable playlist**: the radio playlist can be toggled between newest-first
  and oldest-first via the ⇅ column header; the choice persists across visits.
- **Legacy era stats**: teams that imported a tunesday.json now see a
  "Before tunesday.online" section on the stats page showing pre-migration
  tune count, provider count, and date range.
- **CI + release automation**: GitHub Actions runs the test suite on push
  and PRs; a `v*` tag push now builds the server binary, publishes the
  Docker image to GHCR, and creates a GitHub release with notes from the
  changelog. Pre-release tags (`-rc`, `-beta`, `-pre`) get GitHub-generated
  notes and no `latest` image tag.

### Fixed
- **Radio play stats**: plays are now recorded when reported via WebSocket,
  not just via the fallback POST endpoint. The stats page was perpetually
  showing "0 total plays" because the browser client never called the POST.

## [1.0.2] - 2026-09-07

### Fixed
- Docker build now passes version tag via `-ldflags` so the footer shows the
  actual release version instead of `dev`.

## [1.0.1] - 2026-09-07

### Fixed
- Docker build: correct COPY path for `scripts/backup.sh` (build context is repo root).
- Footer version no longer shows a doubled `v` prefix (`vv1.0.0` → `v1.0.0`).
- Add explicit DNS resolvers (8.8.8.8, 1.1.1.1) to Docker Compose to fix SMTP
  hostname resolution on VPS hosts with non-standard DNS.

### Changed
- Deployment guide now includes firewall setup (ports 80/443) before first launch.

## [1.0.0] - 2026-09-06


_**The tunesday CLI and standalone browser player have been removed.**
  The terminal UI (TUI), the mpv-backed playback path, and the GitHub Pages
  quiz/radio player no longer exist. Migrate your old team by importing your
  `tunesday.json` file — see the [README](README.md)._

### Added

- **Teams & members**: users join teams via magic links (no passwords); admins
  invite members, assign providers, and manage roles.
- **The Tunesday Roulette**: a real-time WebSocket ceremony that fairly (and
  theatrically) picks who provides the week's soundtrack.
- **Team Tunesday settings**: each team sets its own timezone and weekday
  (default Tuesday). Ceremonies only open on the team's Tunesday unless an
  admin overrides.
- **Pull-UP voting**: after a reveal, connected attendees can vote to re-roll.
  If more than half pull up, the ceremony resets to the hanging needle.
- **Radio room**: per-user playback, volume control, provider/date metadata,
  now-playing presence.
- **Quiz**: guess which teammate submitted a tune from short snippets, with
  persisted leaderboards.
- **Stats**: tune of the week, play counts, ceremony attendance, quiz leaderboards.
- **Master admin**: a violator of all team membership checks, with an
  all-teams dashboard.
- **Email reminders**: only the ceremony winner is emailed if no tune lands on
  their Tunesday.
- **Resume or cancel a ceremony**: an open ceremony (needle hanging) can be
  picked back up later, or cancelled by an admin.
- **Late tune provision**: the winner (or an admin) can register a revealed
  ceremony's tune from the dashboard long after the needle dropped.
- **Automatic backups**: a Docker sidecar snapshots the SQLite DB daily
  (`VACUUM INTO`, WAL-safe) with retention and a restore runbook.
- **Versioned footer**: the footer shows the build tag/commit.

### Changed

- The ceremony pool is now **all eligible providers connected to the room**
  with no last-submitter exclusion; re-selection is handled by team vote
  (Pull-UP) rather than an automatic exclusion.
- Deployment is now a single web service in Docker behind Caddy; the
  Makefile builds only the server.

[1.0.3]: https://github.com/daum3ns/tunesday/releases/tag/v1.0.3
[1.0.2]: https://github.com/daum3ns/tunesday/releases/tag/v1.0.2
[1.0.1]: https://github.com/daum3ns/tunesday/releases/tag/v1.0.1
[1.0.0]: https://github.com/daum3ns/tunesday/releases/tag/v1.0.0
