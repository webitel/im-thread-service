-- +goose Up
-- +goose StatementBegin
-- Covering index for the SearchLeft CTE: streams membership periods pre-sorted
-- by left_at DESC and avoids heap fetches.
CREATE INDEX IF NOT EXISTS idx_thread_dialog_member_left_at
    ON im_thread.thread_dialog (member_id, deleted_at DESC)
    INCLUDE (id, thread_id, created_at)
    WHERE deleted_at IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS im_thread.idx_thread_dialog_member_left_at;
-- +goose StatementEnd
