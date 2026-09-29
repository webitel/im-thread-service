-- +goose Up
-- GetUpdates replays a short overlap before the cursor's own change: when marks last moved,
-- and the journal time of a transaction.
alter table "im_thread"."contact_updates"
    add column if not exists "updated_at" timestamptz not null default clock_timestamp();

create index if not exists "idx_thread_updates_tx"
    on "im_message"."thread_updates" ("tx_id");

-- +goose Down
drop index if exists "im_message"."idx_thread_updates_tx";
alter table "im_thread"."contact_updates" drop column if exists "updated_at";
