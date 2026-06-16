-- +goose Up

-- Monotonic counter incremented on every bot control grant (Push or Pop).
-- Included in bot.control.granted.v1 events and required in CompleteBotControl
-- to prevent stale/duplicate requests from being accepted (ABA protection).
ALTER TABLE im_thread.thread
    ADD COLUMN control_epoch bigint NOT NULL DEFAULT 0;

-- Prevent the same bot from occupying multiple positions in the stack simultaneously.
ALTER TABLE im_thread.bot_control_stack
    ADD CONSTRAINT bot_control_stack_thread_member_unique UNIQUE (thread_id, member_id);

-- +goose Down
ALTER TABLE im_thread.thread DROP COLUMN control_epoch;
ALTER TABLE im_thread.bot_control_stack DROP CONSTRAINT bot_control_stack_thread_member_unique;
