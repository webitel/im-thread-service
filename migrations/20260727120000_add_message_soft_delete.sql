-- +goose Up
alter table "im_message"."messages"
    add column if not exists "deleted_at" timestamptz,
    add column if not exists "deleted_by" uuid;

create index if not exists idx_messages_deleted_at
    on im_message.messages (thread_id, deleted_at)
    where deleted_at is not null;

drop view if exists "im_thread"."v_messages";

-- Rebuilds v_messages with:
--   deleted_at/deleted_by    - soft-delete marks.
--   edited                   - the column exists since 20260709121745 but was
--                              never exposed here, so model.Message.Edited
--                              could not be populated from history.
--   delivery_status/statuses - restored: 20260715090001 added them and the
--                              later 20260716120000 view rebuild dropped them,
--                              while query_object/message_history.go still
--                              selects them.
--   reply_to                 - never leaks the body of a deleted quote target.
create or replace view "im_thread"."v_messages" as (
SELECT m.id,
    m.thread_id,
    m.sender_id,
    m.type,
    m.body,
    m.metadata,
    m.created_at,
    m.updated_at,
    m.edited,
    m.deleted_at,
    m.deleted_by,
	to_jsonb(bc.*) as reacted_metadata,
    ( SELECT jsonb_agg(unnamed_subquery.document) AS jsonb_agg
           FROM ( SELECT jsonb_build_object('id', doc.id, 'message_id', doc.message_id, 'file_id', doc.file_id, 'name', doc.name, 'mime', doc.mime, 'size', doc.size, 'created_at', doc.created_at) AS document
                   FROM im_message.message_documents doc
                  WHERE (m.type = 2 OR m.type = 5 AND ((m.interactive -> 'attachments'::text) -> 'documents'::text) IS NOT NULL) AND doc.message_id = m.id) unnamed_subquery) AS documents,
    ( SELECT jsonb_agg(unnamed_subquery.image) AS jsonb_agg
           FROM ( SELECT jsonb_build_object('id', img.id, 'message_id', img.message_id, 'file_id', img.file_id, 'mime', img.mime, 'width', img.width, 'height', img.height, 'created_at', img.created_at) AS image
                   FROM im_message.message_images img
                  WHERE (m.type = 3 OR m.type = 5 AND ((m.interactive -> 'attachments'::text) -> 'images'::text) IS NOT NULL) AND img.message_id = m.id) unnamed_subquery) AS images,
    ( SELECT jsonb_build_object('name', mc.name, 'phone_number', mc.phone_number, 'email', mc.email) AS jsonb_build_object
           FROM im_message.message_contacts mc
          WHERE m.type = 7 AND m.id = mc.message_id
         LIMIT 1) AS contact,
    ( SELECT jsonb_build_object('address', ml.address, 'name', ml.name, 'latitude', ml.latitude, 'longitude', ml.longitude) AS jsonb_build_object
           FROM im_message.message_locations ml
          WHERE m.type = 6 AND m.id = ml.message_id
         LIMIT 1) AS location,
    m.domain_id,
    tm.member_data AS member,
    m.interactive,
    ( SELECT jsonb_build_object('type', sm.type, 'metadata', sm.metadata) AS jsonb_build_object
           FROM im_message.system_messages sm
          WHERE m.type = 4 AND sm.message_id = m.id
         LIMIT 1) AS system,
    ( SELECT jsonb_build_object(
            'message_id', r.id,
            'sender_id', r.sender_id,
            'type', r.type,
            'body', CASE WHEN r.deleted_at IS NOT NULL THEN '' ELSE left(coalesce(r.body, ''), 256) END,
            'created_at', (extract(epoch from r.created_at) * 1000)::bigint,
            'deleted', (r.deleted_at IS NOT NULL),
            'attachment', CASE WHEN r.deleted_at IS NOT NULL THEN NULL ELSE coalesce(
                ( SELECT jsonb_build_object('kind', 'document', 'name', d.name, 'mime', d.mime)
                    FROM im_message.message_documents d
                   WHERE d.message_id = r.id
                   ORDER BY d.created_at
                   LIMIT 1),
                ( SELECT jsonb_build_object('kind', 'image', 'mime', i.mime)
                    FROM im_message.message_images i
                   WHERE i.message_id = r.id
                   ORDER BY i.created_at
                   LIMIT 1)) END)
       FROM im_message.messages r
      WHERE r.id = m.reply_to) AS reply_to,
    ( SELECT CASE
                WHEN bool_and(st.status = 4) THEN 4
                ELSE min(st.status) FILTER (WHERE st.status <> 4)
             END
           FROM im_message.message_statuses st
          WHERE st.message_id = m.id) AS delivery_status,
    ( SELECT jsonb_agg(jsonb_build_object(
                'member_id', st.member_id,
                'status', st.status,
                'delivered_at', st.delivered_at,
                'read_at', st.read_at,
                'failed_at', st.failed_at,
                'via', st.via,
                'error', st.error
             ) ORDER BY st.updated_at)
           FROM im_message.message_statuses st
          WHERE st.message_id = m.id) AS statuses
   FROM im_message.messages m
     LEFT JOIN LATERAL ( SELECT jsonb_build_object('id', td.id, 'role', td.thread_role, 'member_id', m.sender_id) AS member_data
           FROM im_thread.thread_dialog td
          WHERE td.member_id = m.sender_id AND td.thread_id = m.thread_id
          ORDER BY td.id DESC
         LIMIT 1) tm ON true
   left join im_message.buttons_callback bc on m.type = 5 and bc.in_reply_to = m.id
);

-- +goose Down
drop view if exists "im_thread"."v_messages";

create or replace view "im_thread"."v_messages" as (
SELECT m.id,
    m.thread_id,
    m.sender_id,
    m.type,
    m.body,
    m.metadata,
    m.created_at,
    m.updated_at,
	to_jsonb(bc.*) as reacted_metadata,
    ( SELECT jsonb_agg(unnamed_subquery.document) AS jsonb_agg
           FROM ( SELECT jsonb_build_object('id', doc.id, 'message_id', doc.message_id, 'file_id', doc.file_id, 'name', doc.name, 'mime', doc.mime, 'size', doc.size, 'created_at', doc.created_at) AS document
                   FROM im_message.message_documents doc
                  WHERE (m.type = 2 OR m.type = 5 AND ((m.interactive -> 'attachments'::text) -> 'documents'::text) IS NOT NULL) AND doc.message_id = m.id) unnamed_subquery) AS documents,
    ( SELECT jsonb_agg(unnamed_subquery.image) AS jsonb_agg
           FROM ( SELECT jsonb_build_object('id', img.id, 'message_id', img.message_id, 'file_id', img.file_id, 'mime', img.mime, 'width', img.width, 'height', img.height, 'created_at', img.created_at) AS image
                   FROM im_message.message_images img
                  WHERE (m.type = 3 OR m.type = 5 AND ((m.interactive -> 'attachments'::text) -> 'images'::text) IS NOT NULL) AND img.message_id = m.id) unnamed_subquery) AS images,
    ( SELECT jsonb_build_object('name', mc.name, 'phone_number', mc.phone_number, 'email', mc.email) AS jsonb_build_object
           FROM im_message.message_contacts mc
          WHERE m.type = 7 AND m.id = mc.message_id
         LIMIT 1) AS contact,
    ( SELECT jsonb_build_object('address', ml.address, 'name', ml.name, 'latitude', ml.latitude, 'longitude', ml.longitude) AS jsonb_build_object
           FROM im_message.message_locations ml
          WHERE m.type = 6 AND m.id = ml.message_id
         LIMIT 1) AS location,
    m.domain_id,
    tm.member_data AS member,
    m.interactive,
    ( SELECT jsonb_build_object('type', sm.type, 'metadata', sm.metadata) AS jsonb_build_object
           FROM im_message.system_messages sm
          WHERE m.type = 4 AND sm.message_id = m.id
         LIMIT 1) AS system,
    ( SELECT jsonb_build_object(
            'message_id', r.id,
            'sender_id', r.sender_id,
            'type', r.type,
            'body', left(coalesce(r.body, ''), 256),
            'created_at', (extract(epoch from r.created_at) * 1000)::bigint,
            'attachment', coalesce(
                ( SELECT jsonb_build_object('kind', 'document', 'name', d.name, 'mime', d.mime)
                    FROM im_message.message_documents d
                   WHERE d.message_id = r.id
                   ORDER BY d.created_at
                   LIMIT 1),
                ( SELECT jsonb_build_object('kind', 'image', 'mime', i.mime)
                    FROM im_message.message_images i
                   WHERE i.message_id = r.id
                   ORDER BY i.created_at
                   LIMIT 1)))
       FROM im_message.messages r
      WHERE r.id = m.reply_to) AS reply_to
   FROM im_message.messages m
     LEFT JOIN LATERAL ( SELECT jsonb_build_object('id', td.id, 'role', td.thread_role, 'member_id', m.sender_id) AS member_data
           FROM im_thread.thread_dialog td
          WHERE td.member_id = m.sender_id AND td.thread_id = m.thread_id
          ORDER BY td.id DESC
         LIMIT 1) tm ON true
   left join im_message.buttons_callback bc on m.type = 5 and bc.in_reply_to = m.id
);

drop index if exists im_message.idx_messages_deleted_at;

alter table "im_message"."messages"
    drop column if exists "deleted_at",
    drop column if exists "deleted_by";
