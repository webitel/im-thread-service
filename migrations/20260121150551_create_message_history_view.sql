-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE VIEW im_thread.v_messages AS
SELECT id,
    thread_id,
    sender_id,
    type,
    body,
    metadata,
    created_at,
    updated_at,
    (
        SELECT jsonb_agg(unnamed_subquery.document) AS jsonb_agg
        FROM (
                SELECT jsonb_build_object(
                        'id',
                        doc.id,
                        'message_id',
                        doc.message_id,
                        'file_id',
                        doc.file_id,
                        'name',
                        doc.name,
                        'mime',
                        doc.mime,
                        'size',
                        doc.size,
                        'created_at',
                        doc.created_at
                    ) AS document
                FROM im_message.message_documents doc
                WHERE m.type = 2
                AND doc.message_id = m.id
            ) unnamed_subquery
    ) AS documents,
    (
        SELECT jsonb_agg(unnamed_subquery.image) AS jsonb_agg
        FROM (
                SELECT jsonb_build_object(
                        'id',
                        img.id,
                        'message_id',
                        img.message_id,
                        'file_id',
                        img.file_id,
                        'mime',
                        img.mime,
                        'width',
                        img.width,
                        'height',
                        img.height,
                        'created_at',
                        img.created_at
                    ) AS image
                FROM im_message.message_images img
                WHERE m.type = 3
                AND img.message_id = m.id
            ) unnamed_subquery
    ) AS images,
    domain_id
FROM im_message.messages m;
create index if not exists idx_messages_pagination_desc ON im_message.messages using btree(created_at DESC, id DESC);
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
drop view if exists im_thread.v_messages;
drop index if exists idx_messages_pagination_desc;
-- +goose StatementEnd
