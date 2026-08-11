-- +goose Up

-- internal marks a message as an operator-only note: visible to Webitel users,
-- never delivered to the client contact and never forwarded to an external
-- messenger. Enforced in the service/read layers.
--
-- Renumbered ABOVE 20260805134353_add_message_forward_origin and
-- 20260805144532_add_message_delivery_state on merge: that forward_origin
-- migration drops+recreates v_messages without the internal column, so this
-- must run last and rebuild the view from the latest definition + m.internal.
alter table "im_message"."messages"
    add column if not exists "internal" boolean not null default false;

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
    m.edited,
    m.deleted_at,
    m.deleted_by,
    m.internal,
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
    CASE WHEN m.forward_origin_kind IS NULL THEN NULL ELSE
        jsonb_build_object(
            'kind',              m.forward_origin_kind,
            'sender_id',         m.forward_origin_sender_id,
            'sender_name',       coalesce(m.forward_origin_sender_name, ''),
            'original_sent_at',  (extract(epoch from m.forward_origin_sent_at) * 1000)::bigint,
            'source_message_id', m.forward_from_message_id
        ) END AS forward_origin,
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
          WHERE st.message_id = m.id) AS statuses,
    ( SELECT jsonb_agg(jsonb_build_object(
                'emoji', e.emoji,
                'count', e.cnt,
                'reactor_ids', e.reactor_ids,
                'last_reacted_at', e.last_ms
             ) ORDER BY e.first_at)
           FROM ( SELECT mr.emoji,
                         count(*)::int AS cnt,
                         to_jsonb((array_agg(mr.reactor_id ORDER BY mr.created_at))[1:12]) AS reactor_ids,
                         min(mr.created_at) AS first_at,
                         (extract(epoch from max(mr.updated_at)) * 1000)::bigint AS last_ms
                    FROM im_message.message_reactions mr
                   WHERE mr.message_id = m.id
                   GROUP BY mr.emoji ) e ) AS reactions
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

-- Restores the 20260805134353 definition (reactions + forward_origin, no internal).
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
    CASE WHEN m.forward_origin_kind IS NULL THEN NULL ELSE
        jsonb_build_object(
            'kind',              m.forward_origin_kind,
            'sender_id',         m.forward_origin_sender_id,
            'sender_name',       coalesce(m.forward_origin_sender_name, ''),
            'original_sent_at',  (extract(epoch from m.forward_origin_sent_at) * 1000)::bigint,
            'source_message_id', m.forward_from_message_id
        ) END AS forward_origin,
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
          WHERE st.message_id = m.id) AS statuses,
    ( SELECT jsonb_agg(jsonb_build_object(
                'emoji', e.emoji,
                'count', e.cnt,
                'reactor_ids', e.reactor_ids,
                'last_reacted_at', e.last_ms
             ) ORDER BY e.first_at)
           FROM ( SELECT mr.emoji,
                         count(*)::int AS cnt,
                         to_jsonb((array_agg(mr.reactor_id ORDER BY mr.created_at))[1:12]) AS reactor_ids,
                         min(mr.created_at) AS first_at,
                         (extract(epoch from max(mr.updated_at)) * 1000)::bigint AS last_ms
                    FROM im_message.message_reactions mr
                   WHERE mr.message_id = m.id
                   GROUP BY mr.emoji ) e ) AS reactions
   FROM im_message.messages m
     LEFT JOIN LATERAL ( SELECT jsonb_build_object('id', td.id, 'role', td.thread_role, 'member_id', m.sender_id) AS member_data
           FROM im_thread.thread_dialog td
          WHERE td.member_id = m.sender_id AND td.thread_id = m.thread_id
          ORDER BY td.id DESC
         LIMIT 1) tm ON true
   left join im_message.buttons_callback bc on m.type = 5 and bc.in_reply_to = m.id
);

alter table "im_message"."messages"
    drop column if exists "internal";
