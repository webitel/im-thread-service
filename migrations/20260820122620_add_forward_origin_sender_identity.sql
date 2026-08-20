-- +goose Up
alter table "im_message"."messages"
    add column if not exists "forward_origin_sender_iss" text,
    add column if not exists "forward_origin_sender_sub" text;

-- +goose Down
alter table "im_message"."messages"
    drop column if exists "forward_origin_sender_iss",
    drop column if exists "forward_origin_sender_sub";
