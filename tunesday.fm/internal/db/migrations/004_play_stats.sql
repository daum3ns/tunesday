-- Radio play log: one row per track start. Feeds "tune of the week" and
-- lets the team see what actually got listened to.
CREATE TABLE play_stats (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    team_id         TEXT NOT NULL REFERENCES teams(id),
    tune_id         INTEGER NOT NULL REFERENCES tunes(id),
    user_id         TEXT REFERENCES users(id),
    room_session_id TEXT NOT NULL,
    started_at      TEXT DEFAULT (datetime('now'))
);

CREATE INDEX idx_play_stats_team ON play_stats(team_id, started_at);
