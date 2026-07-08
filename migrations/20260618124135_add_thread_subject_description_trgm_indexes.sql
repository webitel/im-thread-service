-- +goose NO TRANSACTION

-- +goose Up

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_thread_subject_trgm
    ON im_thread.thread USING gin (subject gin_trgm_ops);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_thread_description_trgm
    ON im_thread.thread USING gin (description gin_trgm_ops);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_direct_settings_title_trgm
    ON im_thread.direct_settings USING gin (title gin_trgm_ops);

-- +goose Down

DROP INDEX CONCURRENTLY IF EXISTS im_thread.idx_thread_subject_trgm;
DROP INDEX CONCURRENTLY IF EXISTS im_thread.idx_thread_description_trgm;
DROP INDEX CONCURRENTLY IF EXISTS im_thread.idx_direct_settings_title_trgm;
