-- +goose Up
-- +goose StatementBegin
CREATE SCHEMA IF NOT EXISTS im_message;

-- Main table for storing messages
CREATE TABLE im_message.messages (
    -- uuidv7 is preferred for primary keys due to sequential nature and time-sorting
    id UUID PRIMARY KEY DEFAULT uuidv7(), 
    thread_id UUID NOT NULL,
    from_id UUID NOT NULL,
    to_id UUID NOT NULL,
    body TEXT NOT NULL,
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Transactional Outbox table for reliable event delivery
CREATE TABLE im_message.messages_outbox (
    "offset" BIGSERIAL,
    "uuid" UUID NOT NULL,
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT now(),
    "payload" BYTEA DEFAULT NULL,
    -- JSONB provides better performance and indexing capabilities than plain JSON
    "metadata" JSONB DEFAULT NULL,
    -- xid8 ensures transaction consistency in PostgreSQL for Watermill SQL Subscriber
    "transaction_id" xid8 NOT NULL DEFAULT pg_current_xact_id(),
    PRIMARY KEY ("transaction_id", "offset")
);

-- Index for efficient background cleanup jobs (retention)
CREATE INDEX idx_messages_outbox_created_at ON im_message.messages_outbox (created_at);

-- Watermill offsets table to track subscriber progress across instances
CREATE TABLE im_message.watermill_offsets (
    consumer_group TEXT NOT NULL,
    topic TEXT NOT NULL,
    -- Using BIGINT to match the BIGSERIAL offset from the outbox table
    offset_value BIGINT,
    PRIMARY KEY (consumer_group, topic)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS im_message.watermill_offsets;
DROP TABLE IF EXISTS im_message.messages_outbox;
DROP TABLE IF EXISTS im_message.messages;
-- Optional: DROP SCHEMA im_message; (Only if this schema is exclusive to this service)
-- +goose StatementEnd