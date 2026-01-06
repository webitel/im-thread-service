-- +goose Up
-- +goose StatementBegin

-- IMAGE ATTACHMENTS (1:N relation)
CREATE TABLE IF NOT EXISTS im_message.message_images (
    id          UUID PRIMARY KEY DEFAULT uuidv7(),
    message_id  UUID NOT NULL REFERENCES im_message.messages(id) ON DELETE CASCADE,
    file_id     BIGINT NOT NULL, 
    name        TEXT,            
    mime        VARCHAR(64),     
    thumbnails  JSONB,           
    width       INT,
    height      INT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- DOCUMENT ATTACHMENTS (1:N relation)
CREATE TABLE IF NOT EXISTS im_message.message_documents (
    id          UUID PRIMARY KEY DEFAULT uuidv7(),
    message_id  UUID NOT NULL REFERENCES im_message.messages(id) ON DELETE CASCADE,
    file_id     BIGINT NOT NULL,  
    name        TEXT NOT NULL,   
    mime        VARCHAR(64),     
    size        BIGINT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_msg_images_message_id ON im_message.message_images(message_id);
CREATE INDEX IF NOT EXISTS idx_msg_docs_message_id ON im_message.message_documents(message_id);

-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS im_message.message_documents;
DROP TABLE IF EXISTS im_message.message_images;