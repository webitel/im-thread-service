-- +goose Up
-- +goose StatementBegin
-- Per-participant read horizon and denormalized unread counter, maintained in
-- the same transactions that change delivery state (recompute-on-write). This
-- mirrors the canonical messenger model (Telegram read_inbox_max_id + dialog
-- unread_count, Slack channel last_read + unread): unread is derived from a
-- read horizon, not from a per-message scan on every read.
--
--   last_read_message_id  the newest message the member has read (read-up-to
--                         boundary); advances monotonically, never backward.
--   unread_count          content messages after the horizon not sent by the
--                         member; kept in sync on new message (+1) and on read.
ALTER TABLE im_thread.thread_dialog
    ADD COLUMN IF NOT EXISTS last_read_message_id UUID,
    ADD COLUMN IF NOT EXISTS unread_count INTEGER NOT NULL DEFAULT 0;

-- Backfill the horizon from existing per-message read state: the newest message
-- a member has read (status = 3). Message ids are UUIDv7 (time-ordered) and the
-- uuid type has btree ordering, so the greatest id is the latest read. Postgres
-- has no max(uuid) aggregate, so pick it with DISTINCT ON + ORDER BY ... DESC.
UPDATE im_thread.thread_dialog td
SET last_read_message_id = h.max_read
FROM (
    SELECT DISTINCT ON (thread_id, member_id)
           thread_id, member_id, message_id AS max_read
    FROM im_message.message_statuses
    WHERE status = 3
    ORDER BY thread_id, member_id, message_id DESC
) h
WHERE h.thread_id = td.thread_id AND h.member_id = td.member_id;

-- Backfill unread_count from the horizon: content messages (type <> 4 system)
-- after the horizon that the member did not send.
UPDATE im_thread.thread_dialog td
SET unread_count = coalesce((
    SELECT count(*)
    FROM im_message.messages m
    WHERE m.thread_id = td.thread_id
      AND m.sender_id <> td.member_id
      AND m.type <> 4
      AND (td.last_read_message_id IS NULL OR m.id > td.last_read_message_id)
), 0);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE im_thread.thread_dialog
    DROP COLUMN IF EXISTS unread_count,
    DROP COLUMN IF EXISTS last_read_message_id;
-- +goose StatementEnd
