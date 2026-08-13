-- +goose Up

-- Delivered-side twin of last_read_message_id (20260727120001): a monotonic
-- per-(thread,member) watermark. A recipient that received message X implicitly
-- received everything before it, so the ws/push delivered state collapses from
-- one message_statuses row per (message x recipient) to this single column.
-- Nullable, no default -> instant on a large table.
alter table "im_thread"."thread_dialog"
    add column if not exists "last_delivered_message_id" uuid;

-- Backfill from the existing per-message table: the highest message each member
-- has at least DELIVERED. status >= 2 covers DELIVERED(2) and READ(3) (read
-- implies delivered); message_id is UUIDv7, so the max id is the horizon.
update "im_thread"."thread_dialog" td
set last_delivered_message_id = h.max_delivered
from (
    select distinct on (thread_id, member_id)
           thread_id, member_id, message_id as max_delivered
    from "im_message"."message_statuses"
    where status >= 2
    order by thread_id, member_id, message_id desc
) h
where h.thread_id = td.thread_id
  and h.member_id = td.member_id;

-- +goose Down
alter table "im_thread"."thread_dialog"
    drop column if exists "last_delivered_message_id";
