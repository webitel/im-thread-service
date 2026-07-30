-- +goose Up
-- +goose StatementBegin
-- DEFAULT TRUE keeps existing members able to delete their own messages: the
-- right is on by default and revoked per member, like can_send_messages.
ALTER TABLE im_thread.thread_permission
    ADD COLUMN IF NOT EXISTS can_delete_messages BOOLEAN NOT NULL DEFAULT TRUE;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE im_thread.thread_permission
    DROP COLUMN IF EXISTS can_delete_messages;
-- +goose StatementEnd
