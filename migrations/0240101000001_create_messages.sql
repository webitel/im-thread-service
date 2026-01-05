-- +goose Up
-- +goose StatementBegin

CREATE SCHEMA IF NOT EXISTS im_message;

-- MESSAGES
CREATE TABLE IF NOT EXISTS im_message.messages (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    thread_id UUID NOT NULL,
    from_id   UUID NOT NULL,
    from_type SMALLINT NOT NULL DEFAULT 0,
    to_id     UUID NOT NULL,
    to_type   SMALLINT NOT NULL DEFAULT 0,
    body TEXT NOT NULL,
    type SMALLINT NOT NULL DEFAULT 0,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- OUTBOX
CREATE TABLE IF NOT EXISTS im_message.messages_outbox (
    "offset" BIGSERIAL,
    "uuid" VARCHAR(36) NOT NULL,
    "created_at" TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "payload" BYTEA DEFAULT NULL,
    "metadata" JSON DEFAULT NULL,
    "transaction_id" xid8 NOT NULL DEFAULT pg_current_xact_id(),
    PRIMARY KEY ("transaction_id", "offset")
);

-- WATERMILL OFFSETS
DROP TABLE IF EXISTS im_message.messages_offsets;

CREATE TABLE im_message.messages_offsets (
    consumer_group TEXT NOT NULL,
    topic TEXT NOT NULL DEFAULT 'im.messages', 
    offset_acked BIGINT NOT NULL DEFAULT 0,
    last_processed_transaction_id xid8 NOT NULL DEFAULT '0'::xid8,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer_group)
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP SCHEMA IF EXISTS im_message CASCADE;
-- +goose StatementEnd