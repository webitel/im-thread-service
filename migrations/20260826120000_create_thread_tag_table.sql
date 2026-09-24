-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS im_thread.thread_tag (
    "id" uuid DEFAULT uuidv7() PRIMARY KEY,
    "thread_id" uuid NOT NULL,
    "contact_id" uuid NOT NULL,
    "tag" text NOT NULL CHECK (char_length(tag) BETWEEN 1 AND 64),
    "created_at" timestamptz NOT NULL DEFAULT NOW(),
    FOREIGN KEY (thread_id) REFERENCES im_thread.thread(id) ON DELETE CASCADE,
    UNIQUE(thread_id, contact_id, tag)
);

CREATE INDEX idx_thread_tag_contact_thread ON im_thread.thread_tag(contact_id, thread_id);
-- Backs the search filter's WHERE contact_id = ? AND tag IN (...) predicate.
CREATE INDEX idx_thread_tag_contact_tag ON im_thread.thread_tag(contact_id, tag);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS im_thread.thread_tag;
-- +goose StatementEnd
