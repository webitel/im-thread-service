-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE VIEW im_thread.v_messages AS
select m.id,
    m.thread_id,
    m.sender_id,
    m.receiver_id,
    m.type,
    m.body,
    m.metadata,
    m.created_at,
    m.updated_at,
    (
        select jsonb_agg("document")
        from (
                select jsonb_build_object(
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
                    ) as "document"
                from im_message.message_documents doc
                where doc.message_id = m.id
            )
    ) as documents,
    (
        select jsonb_agg(image)
        from (
                select jsonb_build_object(
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
                    ) as image
                from im_message.message_images img
                where img.message_id = m.id
            )
    ) as images
from im_message.messages m;
create index if not exists idx_messages_pagination_desc ON im_message.messages using btree(created_at DESC, id DESC);
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
drop view if exists im_thread.v_messages;
drop index if exists idx_messages_pagination_desc;
-- +goose StatementEnd