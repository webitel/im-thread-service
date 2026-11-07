-- +goose NO TRANSACTION

-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_thread_dialog_thread_member
    ON im_thread.thread_dialog (thread_id, member_id)
    INCLUDE (created_at, deleted_at);

-- +goose Down

DROP INDEX CONCURRENTLY IF EXISTS im_thread.idx_thread_dialog_thread_member;
