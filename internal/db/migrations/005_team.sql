-- Blitzball Labs Analytics — migration 005
-- CloseCrab Team Mode: cloud leaderboard + online presence, and a per-license
-- "remote control enabled" switch the customer can toggle from the portal.
--
-- A "team" is all the phone clients served by one activated CloseCrab license
-- (one GPU host serves the whole team). team_id is derived server-side as the
-- first 16 hex chars of SHA-256(license_key) so the raw key never travels with
-- score traffic.

CREATE TABLE IF NOT EXISTS team_scores (
    id          BIGSERIAL PRIMARY KEY,
    team_id     TEXT NOT NULL,                       -- SHA-256(key)[:16]
    username    TEXT NOT NULL,
    device_id   TEXT NOT NULL DEFAULT '',            -- contributing host fingerprint
    score       BIGINT NOT NULL DEFAULT 0,
    badges      JSONB NOT NULL DEFAULT '[]'::jsonb,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (team_id, username)
);
CREATE INDEX IF NOT EXISTS idx_team_scores_board ON team_scores (team_id, score DESC);

-- Presence: a row per (team, username) refreshed by heartbeats; "online" means
-- last_seen within the freshness window (computed in the query, not stored).
CREATE TABLE IF NOT EXISTS team_presence (
    team_id     TEXT NOT NULL,
    username    TEXT NOT NULL,
    last_seen   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (team_id, username)
);
CREATE INDEX IF NOT EXISTS idx_team_presence_seen ON team_presence (team_id, last_seen DESC);

-- Per-license remote-control switch. Default true so existing licenses keep
-- working; the customer can flip it off from the account portal to kill any
-- phone-remote access bound to that key.
ALTER TABLE licenses
    ADD COLUMN IF NOT EXISTS remote_enabled BOOLEAN NOT NULL DEFAULT true;
