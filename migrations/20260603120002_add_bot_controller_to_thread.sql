-- +goose Up
alter table "im_thread"."thread"
    add column if not exists "bot_controller_id" uuid references "im_thread"."thread_dialog" ("id") on delete set null;

-- +goose Down
alter table "im_thread"."thread"
    drop column if exists "bot_controller_id";
