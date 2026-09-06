ALTER TABLE teams ADD COLUMN timezone TEXT NOT NULL DEFAULT 'UTC';
ALTER TABLE teams ADD COLUMN tunesday_weekday INTEGER NOT NULL DEFAULT 2;

-- Tracks that a team's no-tune reminder was sent for a given date, so
-- the midnight email fires once per Tunesday, not every restart.
CREATE TABLE reminder_sent (
    team_id TEXT NOT NULL REFERENCES teams(id),
    for_date TEXT NOT NULL,
    PRIMARY KEY (team_id, for_date)
);