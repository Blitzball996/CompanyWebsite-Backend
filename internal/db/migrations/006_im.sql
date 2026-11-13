-- CloseCrab IM foundation.  Content and relationships are separate from the
-- desktop Agent bridge; Agent access is granted explicitly per conversation.
CREATE TABLE IF NOT EXISTS im_conversations (
    id UUID PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('direct','group')),
    title TEXT NOT NULL DEFAULT '',
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS im_members (
    conversation_id UUID NOT NULL REFERENCES im_conversations(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('owner','admin','member')),
    last_read_cursor BIGINT NOT NULL DEFAULT 0,
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (conversation_id,user_id)
);
CREATE INDEX IF NOT EXISTS idx_im_members_user ON im_members(user_id,joined_at DESC);
CREATE TABLE IF NOT EXISTS im_messages (
    conversation_id UUID NOT NULL REFERENCES im_conversations(id) ON DELETE CASCADE,
    cursor BIGSERIAL,
    id UUID NOT NULL,
    sender_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    kind TEXT NOT NULL CHECK (kind IN ('text','image','video','audio','file','system')),
    body TEXT NOT NULL DEFAULT '',
    reply_to UUID,
    idempotency_key UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    PRIMARY KEY (conversation_id,cursor), UNIQUE(conversation_id,id),
    UNIQUE(sender_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_im_messages_replay ON im_messages(conversation_id,cursor);
CREATE TABLE IF NOT EXISTS im_friendships (
    requester_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    addressee_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    state TEXT NOT NULL CHECK (state IN ('pending','accepted','blocked','rejected')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY(requester_id,addressee_id), CHECK(requester_id<>addressee_id)
);
CREATE INDEX IF NOT EXISTS idx_im_friendships_addressee ON im_friendships(addressee_id,state);
CREATE TABLE IF NOT EXISTS im_agent_grants (
    id UUID PRIMARY KEY,
    conversation_id UUID NOT NULL REFERENCES im_conversations(id) ON DELETE CASCADE,
    granted_by BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
    message_from BIGINT,
    message_to BIGINT,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_im_agent_grants_active ON im_agent_grants(conversation_id,expires_at) WHERE revoked_at IS NULL;
