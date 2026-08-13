-- +goose Up

-- Per-thread monotonic sequence for messages. Gives the frontend a small ordered
-- integer per thread (unlike the UUIDv7 id), so it can express "delivered/read up
-- to seq N per member" cleanly. Assigned on insert from a per-thread counter
-- (im_thread.thread.last_seq); backfilled here for existing rows by creation order.
alter table "im_thread"."thread"
    add column if not exists "last_seq" bigint not null default 0;

-- Nullable for now: the app will assign seq on insert in a follow-up; keeping it
-- nullable avoids breaking the current insert path until that lands.
alter table "im_message"."messages"
    add column if not exists "seq" bigint;

-- Backfill per-thread seq by creation order (created_at, then id as a tiebreaker).
with ranked as (
    select id, row_number() over (partition by thread_id order by created_at, id) as rn
    from "im_message"."messages"
)
update "im_message"."messages" m
set seq = r.rn
from ranked r
where r.id = m.id;

-- Seed each thread's counter to its current max seq.
update "im_thread"."thread" t
set last_seq = coalesce((select max(m.seq) from "im_message"."messages" m where m.thread_id = t.id), 0);

create index if not exists "idx_messages_thread_seq"
    on "im_message"."messages" ("thread_id", "seq");

-- +goose Down
drop index if exists "im_message"."idx_messages_thread_seq";

alter table "im_message"."messages"
    drop column if exists "seq";

alter table "im_thread"."thread"
    drop column if exists "last_seq";
