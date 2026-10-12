-- +goose Up
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_msg_docs_file_id ON im_message.message_documents (file_id);
CREATE INDEX IF NOT EXISTS idx_msg_images_file_id ON im_message.message_images (file_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS im_message.idx_msg_docs_file_id;
DROP INDEX IF EXISTS im_message.idx_msg_images_file_id;
-- +goose StatementEnd
