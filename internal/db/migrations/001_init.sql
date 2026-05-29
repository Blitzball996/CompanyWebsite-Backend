-- Blitzball Labs Analytics — schema

CREATE TABLE IF NOT EXISTS events (
    id          BIGSERIAL PRIMARY KEY,
    type        TEXT        NOT NULL DEFAULT 'pageview', -- pageview | event
    name        TEXT,                                    -- event name (e.g. download_click, lang_switch)
    visitor_id  TEXT        NOT NULL,                    -- anonymous hash(ip+ua+salt)
    session_id  TEXT        NOT NULL,
    page        TEXT        NOT NULL DEFAULT '/',
    title       TEXT,
    referrer    TEXT,
    utm_source  TEXT,
    utm_medium  TEXT,
    utm_campaign TEXT,
    country     TEXT,
    device      TEXT,        -- mobile | tablet | desktop
    browser     TEXT,
    os          TEXT,
    lang        TEXT,
    screen      TEXT,
    duration_ms BIGINT,
    scroll_pct  INT,
    meta        JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_events_created_at ON events (created_at);
CREATE INDEX IF NOT EXISTS idx_events_type       ON events (type);
CREATE INDEX IF NOT EXISTS idx_events_page       ON events (page);
CREATE INDEX IF NOT EXISTS idx_events_visitor    ON events (visitor_id);
CREATE INDEX IF NOT EXISTS idx_events_session    ON events (session_id);
