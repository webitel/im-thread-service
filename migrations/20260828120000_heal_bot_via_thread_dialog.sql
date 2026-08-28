-- +goose Up
-- Heal threads created by an inbound message where the gate via was persisted on
-- the bot participant instead of the external contact. This mirrors, for already
-- stored rows, the write-side fix in initializeDirectThreadDialogs (via is a gate
-- id and belongs on the external contact, never on the bot).

-- 1. Backfill the external contact's via from the bot, but only for direct-shaped
--    threads with exactly one live non-bot member, so internal agents in group
--    threads are never mistakenly tagged as external recipients.
update im_thread.thread_dialog ext
set via = bot.via
from im_thread.thread_dialog bot
where bot.thread_id = ext.thread_id
  and bot.is_bot
  and bot.via is not null
  and bot.deleted_at is null
  and not ext.is_bot
  and ext.deleted_at is null
  and (ext.via is null or ext.via = '')
  and (
      select count(*)
      from im_thread.thread_dialog m
      where m.thread_id = ext.thread_id
        and not m.is_bot
        and m.deleted_at is null
  ) = 1;

-- 2. A bot must never carry a gate via.
update im_thread.thread_dialog
set via = null
where is_bot
  and via is not null;

-- +goose Down
-- Data-healing migration: the previous (incorrect) placement of via on bot rows
-- cannot be reconstructed and must not be restored. No-op on rollback.
select 1;
