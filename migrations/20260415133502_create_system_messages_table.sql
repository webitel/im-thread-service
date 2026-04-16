-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS im_message.system_messages (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    domain_id int not null check(domain_id > 0),
    thread_id UUID NOT NULL references im_thread.thread(id) on delete no action,
    type VARCHAR(64) NOT NULL,
    body TEXT,
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS im_message.system_messages;
