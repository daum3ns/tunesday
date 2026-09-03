CREATE TABLE verification_tokens (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token      TEXT NOT NULL UNIQUE,
    used       INTEGER DEFAULT 0,
    created_at TEXT DEFAULT (datetime('now')),
    used_at    TEXT
);

CREATE INDEX idx_verification_tokens_token ON verification_tokens(token);
