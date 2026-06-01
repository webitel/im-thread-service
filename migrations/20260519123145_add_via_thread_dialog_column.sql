-- +goose Up
alter table "im_thread"."thread_dialog"
add column if not exists "via" text;

-- +goose Down
alter table "im_thread"."thread_dialog"
drop column if exists "via";
