-- +goose Up
-- +goose StatementBegin
CREATE SCHEMA IF NOT EXISTS im_message;

CREATE TABLE IF NOT EXISTS im_message.messages (
    id          UUID PRIMARY KEY DEFAULT uuidv7(),
    thread_id   UUID NOT NULL,
    sender_id   UUID NOT NULL,
    receiver_id UUID NOT NULL,
    type        SMALLINT NOT NULL DEFAULT 1 
                CONSTRAINT check_message_type CHECK (type BETWEEN 1 AND 4),
    body        TEXT,                        
    metadata    JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_messages_thread_lookup ON im_message.messages (thread_id, created_at DESC);
-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS im_message.messages CASCADE;