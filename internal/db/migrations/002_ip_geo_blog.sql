-- Blitzball Labs Analytics — migration 002
-- Adds real-IP + geolocation columns to events, and a posts table for the blog.

-- ---- events: IP + geo (ip2region: country|region|province|city|isp) ----
ALTER TABLE events ADD COLUMN IF NOT EXISTS ip        TEXT;  -- raw client IP (personal data — see privacy policy)
ALTER TABLE events ADD COLUMN IF NOT EXISTS province  TEXT;  -- 省 / state
ALTER TABLE events ADD COLUMN IF NOT EXISTS city      TEXT;  -- 市 / city (most precise reliable level)
ALTER TABLE events ADD COLUMN IF NOT EXISTS isp       TEXT;  -- 运营商 / ISP

CREATE INDEX IF NOT EXISTS idx_events_country  ON events (country);
CREATE INDEX IF NOT EXISTS idx_events_province ON events (province);
CREATE INDEX IF NOT EXISTS idx_events_city     ON events (city);
CREATE INDEX IF NOT EXISTS idx_events_ip       ON events (ip);

-- ---- blog posts ----
CREATE TABLE IF NOT EXISTS posts (
    id           BIGSERIAL PRIMARY KEY,
    slug         TEXT UNIQUE,                       -- url-friendly id (optional)
    title_zh     TEXT NOT NULL DEFAULT '',
    title_en     TEXT NOT NULL DEFAULT '',
    body_zh      TEXT NOT NULL DEFAULT '',          -- markdown / plain
    body_en      TEXT NOT NULL DEFAULT '',
    cover_url    TEXT,
    published    BOOLEAN NOT NULL DEFAULT true,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_posts_published  ON posts (published);
CREATE INDEX IF NOT EXISTS idx_posts_created_at ON posts (created_at DESC);
