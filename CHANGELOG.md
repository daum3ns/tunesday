# Changelog

All notable changes to tunesday.online are documented here.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

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

[1.0.0]: https://github.com/daum3ns/tunesday/releases/tag/v1.0.0
