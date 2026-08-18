-- +goose NO TRANSACTION

-- +goose Up

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_messages_body_trgm
    ON im_message.messages USING gin (body gin_trgm_ops)
    WHERE deleted_at IS NULL AND body IS NOT NULL AND body <> '';

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_thread_dialog_thread_member
    ON im_thread.thread_dialog (thread_id, member_id)
    INCLUDE (created_at, deleted_at);

-- +goose Down

DROP INDEX CONCURRENTLY IF EXISTS im_thread.idx_thread_dialog_thread_member;
DROP INDEX CONCURRENTLY IF EXISTS im_message.idx_messages_body_trgm;
