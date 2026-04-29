-- +goose Up
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_thread_dialog_member_thread_left
    ON im_thread.thread_dialog (member_id, thread_id, deleted_at DESC)
    INCLUDE (created_at)
    WHERE deleted_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_thread_dialog_active_membership
    ON im_thread.thread_dialog (thread_id, member_id)
    WHERE deleted_at IS NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS im_thread.idx_thread_dialog_active_membership;
DROP INDEX IF EXISTS im_thread.idx_thread_dialog_member_thread_left;
-- +goose StatementEnd
