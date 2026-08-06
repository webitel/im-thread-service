-- +goose Up
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
