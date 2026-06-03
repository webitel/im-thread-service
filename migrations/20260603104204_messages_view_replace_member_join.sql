-- +goose Up
drop view if exists "im_thread"."v_messages";

create or replace view "im_thread"."v_messages"
as
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
                WHERE (m.type = 2 or (m.type=5 and m.interactive->'attachments'->'documents' is not null))
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
                WHERE
				(m.type = 3 or (m.type=5 and m.interactive->'attachments'->'images' is not null))
                AND img.message_id = m.id
            ) unnamed_subquery
    ) AS images,
	(
		select jsonb_build_object(
			'name', mc.name,
			'phone_number', mc.phone_number,
			'email', mc.email
		)
		from im_message.message_contacts mc
		where m.type = 7
		and m.id=mc.message_id
		limit 1
	) as contact,
	(
		select jsonb_build_object(
			'address', ml.address,
			'name', ml.name,
			'latitude', ml.latitude,
			'longitude', ml.longitude
		)
		from im_message.message_locations ml
		where m.type=6 and m.id = ml.message_id
		limit 1
	) as "location",
  domain_id,
	tm.member_data as member,
	m.interactive as interactive,
	(
		select jsonb_build_object(
			'type', sm.type,
			'metadata', sm.metadata
		)
		from im_message.system_messages sm
		where m.type = 4 and sm.message_id=m.id
		limit 1
	) as "system"
FROM im_message.messages m
left join lateral (
	select jsonb_build_object(
		'id', td.id,
		'role', td.thread_role,
		'member_id', m.sender_id
	) as member_data
	from im_thread.thread_dialog td
	where td.member_id = m.sender_id
	and td.thread_id = m.thread_id
	order by td.id desc
	limit 1
) tm on true;

-- +goose Down
drop view if exists "im_thread"."v_messages";

create or replace view "im_thread"."v_messages"
as
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
                WHERE (m.type = 2 or (m.type=5 and m.interactive->'attachments'->'documents' is not null))
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
                WHERE
				(m.type = 3 or (m.type=5 and m.interactive->'attachments'->'images' is not null))
                AND img.message_id = m.id
            ) unnamed_subquery
    ) AS images,
	(
		select jsonb_build_object(
			'name', mc.name,
			'phone_number', mc.phone_number,
			'email', mc.email
		)
		from im_message.message_contacts mc
		where m.type = 7
		and m.id=mc.message_id
		limit 1
	) as contact,
	(
		select jsonb_build_object(
			'address', ml.address,
			'name', ml.name,
			'latitude', ml.latitude,
			'longitude', ml.longitude
		)
		from im_message.message_locations ml
		where m.type=6 and m.id = ml.message_id
		limit 1
	) as "location",
    domain_id,
	tm.member_data as member,
	m.interactive as interactive,
	(
		select jsonb_build_object(
			'type', sm.type,
			'metadata', sm.metadata
		)
		from im_message.system_messages sm
		where m.type = 4 and sm.message_id=m.id
		limit 1
	) as "system"
FROM im_message.messages m
left join lateral (
	select jsonb_build_object(
		'id', td.id,
		'role', td.thread_role,
		'member_id', m.sender_id
	) as member_data
	from im_thread.thread_dialog td
	where td.member_id = m.sender_id
	and td.thread_id = m.thread_id
	and td.deleted_at is null
	order by td.id desc
	limit 1
) tm on true;
