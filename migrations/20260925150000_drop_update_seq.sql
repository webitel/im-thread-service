-- +goose Up
-- GetUpdates has one cursor (the transaction horizon); per-thread update counters are gone.
alter table "im_message"."thread_updates" drop column if exists "update_seq";
alter table "im_thread"."thread" drop column if exists "last_update_seq";

create index if not exists "idx_thread_updates_thread_tx"
    on "im_message"."thread_updates" ("thread_id", "tx_id");

-- +goose Down
drop index if exists "im_message"."idx_thread_updates_thread_tx";
alter table "im_thread"."thread"
    add column if not exists "last_update_seq" bigint not null default 0;
-- Rows written without a seq cannot get one back; the journal is a replay window, so drop them.
delete from "im_message"."thread_updates";
alter table "im_message"."thread_updates"
    add column if not exists "update_seq" bigint not null;
create unique index if not exists "uq_thread_updates_thread_seq"
    on "im_message"."thread_updates" ("thread_id", "update_seq");
