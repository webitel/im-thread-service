-- +goose Up
alter table "im_thread"."thread_dialog"
    add column if not exists "is_bot"       boolean not null default false,
    add column if not exists "auto_leave"   boolean not null default false;

-- +goose Down
alter table "im_thread"."thread_dialog"
    drop column if exists "is_bot",
    drop column if exists "auto_leave";
