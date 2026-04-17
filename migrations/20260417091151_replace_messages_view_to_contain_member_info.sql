-- +goose Up
create or replace view "im_thread"."v_messages" as
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
    domain_id,
	tm.member_data as member
FROM im_message.messages m
cross join lateral (
	select jsonb_build_object(
		'id', td.id,
		'role', td.thread_role
	) as member_data
	from im_thread.thread_dialog td
	where td.member_id = m.sender_id
	and td.thread_id = m.thread_id
	and td.deleted_at is null
	order by td.id desc
	limit 1
) tm;

create index if not exists "idx_thread_dialog_thread_member_id_desc"
on "im_thread"."thread_dialog" using btree("thread_id", "member_id", "id" desc)
include ("thread_role") where "deleted_at" is null;

-- +goose Down
create or replace view as
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

drop index if exists "im_thread"."idx_thread_dialog_thread_member_id_desc";
