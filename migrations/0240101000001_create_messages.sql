-- +goose Up
CREATE SCHEMA IF NOT EXISTS im_message;


CREATE TABLE im_message.messages (
    id UUID PRIMARY KEY DEFAULT uuidv7(), 
    thread_id UUID NOT NULL,
    from_id UUID NOT NULL,
    to_id UUID NOT NULL,
    body TEXT NOT NULL,
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);


CREATE TABLE im_message.messages_outbox (
    "offset" BIGSERIAL,
    "uuid" UUID NOT NULL,
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT now(),
    "payload" BYTEA DEFAULT NULL,
    "metadata" JSON DEFAULT NULL,
    "transaction_id" xid8 NOT NULL DEFAULT pg_current_xact_id(),
    "published_at" TIMESTAMPTZ,
    PRIMARY KEY ("transaction_id", "offset")
);

CREATE INDEX ON im_message.messages_outbox (published_at) WHERE published_at IS NOT NULL;

CREATE TABLE im_message.watermill_offsets (
    consumer_group TEXT NOT NULL,
    topic TEXT NOT NULL,
    offset_value TEXT,
    PRIMARY KEY (consumer_group, topic)
);