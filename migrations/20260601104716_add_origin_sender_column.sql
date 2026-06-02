-- +goose Up
alter table "im_message"."messages" add column if not exists "origin_sender" uuid;

-- +goose Down
alter table "im_message"."messages" drop column if exists "origin_sender";
