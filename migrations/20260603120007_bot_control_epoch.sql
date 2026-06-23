-- +goose Up

-- Prevent the same bot from occupying multiple positions in the stack simultaneously.
ALTER TABLE im_thread.bot_control_stack
    ADD CONSTRAINT bot_control_stack_thread_member_unique UNIQUE (thread_id, member_id);

-- +goose Down
ALTER TABLE im_thread.bot_control_stack DROP CONSTRAINT bot_control_stack_thread_member_unique;
