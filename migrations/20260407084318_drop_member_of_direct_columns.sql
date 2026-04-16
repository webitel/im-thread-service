-- +goose Up
alter table "im_thread"."thread_dialog"
drop column if exists "direct_to",
drop column if exists "member_of",
add column if not exists "deleted_at" timestamp with time zone;

create unique index if not exists "idx_thread_dialog_member_unique" on "im_thread"."thread_dialog" ("member_id", "thread_id") where "deleted_at" is null;

-- +goose Down
alter table "im_thread"."thread_dialog"
add column if not exists "direct_to" uuid,
add column if not exists "member_of" uuid,
drop column if exists "deleted_at";

drop index if exists "im_thread"."idx_thread_dialog_member_unique";
