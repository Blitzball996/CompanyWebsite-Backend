CREATE TABLE IF NOT EXISTS im_attachments (
    id UUID PRIMARY KEY,
    owner_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('image','video','audio','file')),
    object_key TEXT NOT NULL UNIQUE,
    mime_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes > 0 AND size_bytes <= 524288000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS im_posts (
    id UUID PRIMARY KEY,
    author_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body TEXT NOT NULL DEFAULT '',
    audience TEXT NOT NULL CHECK (audience IN ('private','friends','group','public')),
    group_conversation_id UUID REFERENCES im_conversations(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS im_post_attachments (
    post_id UUID NOT NULL REFERENCES im_posts(id) ON DELETE CASCADE,
    attachment_id UUID NOT NULL REFERENCES im_attachments(id) ON DELETE RESTRICT,
    PRIMARY KEY(post_id,attachment_id)
);
CREATE INDEX IF NOT EXISTS idx_im_posts_feed ON im_posts(created_at DESC) WHERE deleted_at IS NULL;
