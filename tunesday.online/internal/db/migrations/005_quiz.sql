-- Quiz "Guess the Provider": persistent scores for the server-integrated
-- quiz. Score is recomputed server-side from rounds; client claims are
-- advisory only.
CREATE TABLE quiz_games (
    id          TEXT PRIMARY KEY,
    team_id     TEXT NOT NULL REFERENCES teams(id),
    user_id     TEXT NOT NULL REFERENCES users(id),
    mode        TEXT NOT NULL,              -- quick | universe | all
    score       INTEGER NOT NULL,
    total       INTEGER NOT NULL,
    started_at  TEXT,
    finished_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE quiz_rounds (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id          TEXT NOT NULL REFERENCES quiz_games(id) ON DELETE CASCADE,
    round_num        INTEGER NOT NULL,
    tune_id          INTEGER REFERENCES tunes(id),
    guessed_provider TEXT,
    was_correct      INTEGER
);

CREATE INDEX idx_quiz_games_team ON quiz_games(team_id, finished_at);
CREATE INDEX idx_quiz_rounds_game ON quiz_rounds(game_id);
