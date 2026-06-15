-- +goose Up
CREATE INDEX IF NOT EXISTS idx_bot_control_stack_member
    ON im_thread.bot_control_stack (thread_id, member_id);

-- +goose Down
DROP INDEX IF EXISTS im_thread.idx_bot_control_stack_member;
