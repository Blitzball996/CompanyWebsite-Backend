-- Blitzball Labs Analytics — migration 003
-- License keys (serial numbers) for online activation, one-key-one-device.

CREATE TABLE IF NOT EXISTS licenses (
    id            BIGSERIAL PRIMARY KEY,
    key           TEXT UNIQUE NOT NULL,           -- full serial, e.g. BDPR-7K3P-9WXM-2QH4-RT8C
    product       TEXT NOT NULL,                  -- BDST / BDPR / CCST / CCPR
    edition       TEXT NOT NULL DEFAULT 'standard', -- standard | pro
    order_id      TEXT,                           -- payment / order reference
    email         TEXT,                           -- buyer email (optional)
    status        TEXT NOT NULL DEFAULT 'active', -- active | revoked
    device_id     TEXT,                           -- bound device fingerprint (one-key-one-device)
    activated_at  TIMESTAMPTZ,
    last_seen_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_licenses_product ON licenses (product);
CREATE INDEX IF NOT EXISTS idx_licenses_status  ON licenses (status);
CREATE INDEX IF NOT EXISTS idx_licenses_order   ON licenses (order_id);
