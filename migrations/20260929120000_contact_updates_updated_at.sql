-- +goose Up
-- When the mark last moved: GetUpdates replays a short overlap for cursors taken from live events.
alter table "im_thread"."contact_updates"
    add column if not exists "updated_at" timestamptz not null default clock_timestamp();

-- +goose Down
alter table "im_thread"."contact_updates" drop column if exists "updated_at";
