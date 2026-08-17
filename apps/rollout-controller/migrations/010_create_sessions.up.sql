-- Opaque bearer session tokens, hashed the same way tenant API keys are
-- (SHA-256, never stored in plaintext). token_hash is the primary key
-- since every lookup is "resolve this cookie value".
CREATE TABLE sessions (
    token_hash  TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at  TIMESTAMPTZ NOT NULL
);

CREATE INDEX sessions_user_id_idx ON sessions(user_id);
