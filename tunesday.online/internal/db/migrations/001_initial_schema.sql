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
    role          TEXT NOT NULL DEFAULT 'member',
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
    id                 TEXT PRIMARY KEY,
    team_id            TEXT NOT NULL REFERENCES teams(id),
    started_by         TEXT NOT NULL REFERENCES users(id),
    token              TEXT NOT NULL UNIQUE,
    seed               INTEGER,
    pool_json          TEXT,
    winner_provider_id INTEGER REFERENCES providers(id),
    algorithm_version  TEXT DEFAULT 'bottom-half-v1',
    started_at         TEXT DEFAULT (datetime('now')),
    revealed_at        TEXT,
    completed_at       TEXT
);

CREATE TABLE ceremony_attendees (
    ceremony_id TEXT NOT NULL REFERENCES ceremonies(id),
    user_id     TEXT NOT NULL REFERENCES users(id),
    alias       TEXT NOT NULL,
    joined_at   TEXT DEFAULT (datetime('now')),
    PRIMARY KEY (ceremony_id, user_id)
);

CREATE INDEX idx_providers_team    ON providers(team_id);
CREATE INDEX idx_tunes_team        ON tunes(team_id);
CREATE INDEX idx_tunes_provider    ON tunes(provider_id);
CREATE INDEX idx_tunes_added_at    ON tunes(added_at);
CREATE INDEX idx_members_token     ON team_members(magic_token);
CREATE INDEX idx_ceremonies_token  ON ceremonies(token);
CREATE INDEX idx_invitations_team  ON invitations(team_id);
