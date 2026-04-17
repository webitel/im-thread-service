-- +goose Up
-- +goose StatementBegin

DROP TABLE IF EXISTS im_message.system_messages;

CREATE TABLE IF NOT EXISTS im_message.system_messages (
  message_id UUID PRIMARY KEY REFERENCES im_message.messages(id) ON DELETE CASCADE,
  type       VARCHAR(64) NOT NULL,
  metadata   JSONB NOT NULL DEFAULT '{}'::jsonb
  );

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS im_message.system_messages;

-- +goose StatementEnd
