-- Blitzball Labs Analytics — migration 004
-- User accounts + sessions (for the customer portal: register/login, view
-- purchased license keys, receipts). Linked to licenses by email.

CREATE TABLE IF NOT EXISTS users (
    id             BIGSERIAL PRIMARY KEY,
    email          TEXT UNIQUE NOT NULL,
    pass_hash      TEXT NOT NULL,                 -- bcrypt
    name           TEXT NOT NULL DEFAULT '',
    email_verified BOOLEAN NOT NULL DEFAULT false,
    verify_token   TEXT,                          -- email-verification token
    reset_token    TEXT,                          -- password-reset token
    reset_expires  TIMESTAMPTZ,                   -- reset token expiry
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_users_email        ON users (lower(email));
CREATE INDEX IF NOT EXISTS idx_users_verify_token ON users (verify_token);
CREATE INDEX IF NOT EXISTS idx_users_reset_token  ON users (reset_token);

CREATE TABLE IF NOT EXISTS sessions (
    token       TEXT PRIMARY KEY,                 -- opaque random session id
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL,
    ip          TEXT,
    ua          TEXT
);
CREATE INDEX IF NOT EXISTS idx_sessions_user    ON sessions (user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions (expires_at);

-- Receipts: one per paid order (issued by the payment webhook).
CREATE TABLE IF NOT EXISTS receipts (
    id          BIGSERIAL PRIMARY KEY,
    order_id    TEXT UNIQUE NOT NULL,
    email       TEXT,
    product     TEXT NOT NULL,
    edition     TEXT NOT NULL DEFAULT 'standard',
    amount      TEXT NOT NULL DEFAULT '',         -- e.g. "199.00"
    currency    TEXT NOT NULL DEFAULT 'CNY',
    license_key TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_receipts_email ON receipts (lower(email));
