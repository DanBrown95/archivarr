-- Authentication: a single admin user plus server-side sessions.
--
-- The app is locked down by default: until a user row exists, the API reports
-- "setup required" and the frontend forces first-run account creation. The
-- table allows more than one row so multi-user remains a future option, but the
-- current UI manages exactly one account.
--
-- Sessions are opaque random tokens stored server-side (revocable on logout /
-- credential change). Timestamps are unix seconds, matching the rest of the schema.

CREATE TABLE users (
    id            INTEGER PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE COLLATE NOCASE,
    password_hash TEXT NOT NULL,                 -- bcrypt
    created_at    INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    updated_at    INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);

CREATE TABLE sessions (
    token      TEXT PRIMARY KEY,                 -- opaque, cryptographically random
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    expires_at INTEGER NOT NULL
);
CREATE INDEX idx_sessions_user    ON sessions(user_id);
CREATE INDEX idx_sessions_expires ON sessions(expires_at);
