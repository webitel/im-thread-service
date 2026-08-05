-- +goose Up
-- Per-gate delivery state for a message we pushed to an external channel. It lives on
-- the external-id record because that is the only row that is unique per (gate,
-- channel message) and is what a channel callback can be resolved against.
alter table "im_message"."message_external_ids"
    add column if not exists "delivered_at" timestamptz,
    add column if not exists "read_at" timestamptz,
    add column if not exists "failed_at" timestamptz,
    add column if not exists "failed_reason" text;

-- +goose Down
alter table "im_message"."message_external_ids"
    drop column if exists "delivered_at",
    drop column if exists "read_at",
    drop column if exists "failed_at",
    drop column if exists "failed_reason";
