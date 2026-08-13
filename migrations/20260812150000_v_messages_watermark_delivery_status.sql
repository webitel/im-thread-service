-- +goose Up

-- Derive delivery_status (aggregate) and statuses (per-recipient) from the
-- read/delivered watermarks on thread_dialog instead of scanning the
-- per-message message_statuses table (which was O(messages x recipients)).
-- Aggregate rule is preserved verbatim: all-failed => 4, else min non-failed.
-- FAILED still comes from a sparse lookup into message_statuses (status=4).
-- statuses no longer carries per-recipient timestamps (unused by clients).
drop view if exists "im_thread"."v_messages";
create view "im_thread"."v_messages" as
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
    to_jsonb(bc.*) AS reacted_metadata,
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
    ( SELECT jsonb_build_object('message_id', r.id, 'sender_id', r.sender_id, 'type', r.type, 'body',
                CASE
                    WHEN r.deleted_at IS NOT NULL THEN ''::text
                    ELSE "left"(COALESCE(r.body, ''::text), 256)
                END, 'created_at', (EXTRACT(epoch FROM r.created_at) * 1000::numeric)::bigint, 'deleted', r.deleted_at IS NOT NULL, 'attachment',
                CASE
                    WHEN r.deleted_at IS NOT NULL THEN NULL::jsonb
                    ELSE COALESCE(( SELECT jsonb_build_object('kind', 'document', 'name', d.name, 'mime', d.mime) AS jsonb_build_object
                       FROM im_message.message_documents d
                      WHERE d.message_id = r.id
                      ORDER BY d.created_at
                     LIMIT 1), ( SELECT jsonb_build_object('kind', 'image', 'mime', i.mime) AS jsonb_build_object
                       FROM im_message.message_images i
                      WHERE i.message_id = r.id
                      ORDER BY i.created_at
                     LIMIT 1))
                END) AS jsonb_build_object
           FROM im_message.messages r
          WHERE r.id = m.reply_to) AS reply_to,
        CASE
            WHEN m.forward_origin_kind IS NULL THEN NULL::jsonb
            ELSE jsonb_build_object('kind', m.forward_origin_kind, 'sender_id', m.forward_origin_sender_id, 'sender_name', COALESCE(m.forward_origin_sender_name, ''::text), 'original_sent_at', (EXTRACT(epoch FROM m.forward_origin_sent_at) * 1000::numeric)::bigint, 'source_message_id', m.forward_from_message_id)
        END AS forward_origin,
    ( SELECT CASE
                WHEN count(*) = 0 THEN NULL::integer
                WHEN bool_and(s.st = 4) THEN 4
                ELSE min(s.st) FILTER (WHERE s.st <> 4)::integer
             END
        FROM ( SELECT CASE
                        WHEN EXISTS (SELECT 1 FROM im_message.message_statuses f
                                      WHERE f.message_id = m.id AND f.member_id = td.member_id AND f.status = 4) THEN 4
                        WHEN td.last_read_message_id IS NOT NULL AND td.last_read_message_id >= m.id THEN 3
                        WHEN td.last_delivered_message_id IS NOT NULL AND td.last_delivered_message_id >= m.id THEN 2
                        ELSE 1
                      END AS st
                 FROM im_thread.thread_dialog td
                WHERE td.thread_id = m.thread_id AND td.member_id <> m.sender_id AND td.deleted_at IS NULL ) s
     ) AS delivery_status,
    ( SELECT jsonb_agg(jsonb_build_object('member_id', td.member_id, 'status',
                CASE
                    WHEN f.status = 4 THEN 4
                    WHEN td.last_read_message_id IS NOT NULL AND td.last_read_message_id >= m.id THEN 3
                    WHEN td.last_delivered_message_id IS NOT NULL AND td.last_delivered_message_id >= m.id THEN 2
                    ELSE 1
                END, 'error', f.error) ORDER BY td.member_id)
        FROM im_thread.thread_dialog td
        LEFT JOIN im_message.message_statuses f
               ON f.message_id = m.id AND f.member_id = td.member_id AND f.status = 4
       WHERE td.thread_id = m.thread_id AND td.member_id <> m.sender_id AND td.deleted_at IS NULL
     ) AS statuses,
    ( SELECT jsonb_agg(jsonb_build_object('emoji', e.emoji, 'count', e.cnt, 'reactor_ids', e.reactor_ids, 'last_reacted_at', e.last_ms) ORDER BY e.first_at) AS jsonb_agg
           FROM ( SELECT mr.emoji,
                    count(*)::integer AS cnt,
                    to_jsonb((array_agg(mr.reactor_id ORDER BY mr.created_at))[1:12]) AS reactor_ids,
                    min(mr.created_at) AS first_at,
                    (EXTRACT(epoch FROM max(mr.updated_at)) * 1000::numeric)::bigint AS last_ms
                   FROM im_message.message_reactions mr
                  WHERE mr.message_id = m.id
                  GROUP BY mr.emoji) e) AS reactions,
    (( SELECT count(*)::integer AS count
           FROM im_message.message_revisions rev
          WHERE rev.message_id = m.id)) +
        CASE
            WHEN m.deleted_at IS NOT NULL THEN 1
            ELSE 0
        END AS revision_count,
    m.internal
   FROM im_message.messages m
     LEFT JOIN LATERAL ( SELECT jsonb_build_object('id', td.id, 'role', td.thread_role, 'member_id', m.sender_id) AS member_data
           FROM im_thread.thread_dialog td
          WHERE td.member_id = m.sender_id AND td.thread_id = m.thread_id
          ORDER BY td.id DESC
         LIMIT 1) tm ON true
     LEFT JOIN im_message.buttons_callback bc ON m.type = 5 AND bc.in_reply_to = m.id;
;

-- +goose Down
drop view if exists "im_thread"."v_messages";
create view "im_thread"."v_messages" as
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
    to_jsonb(bc.*) AS reacted_metadata,
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
    ( SELECT jsonb_build_object('message_id', r.id, 'sender_id', r.sender_id, 'type', r.type, 'body',
                CASE
                    WHEN r.deleted_at IS NOT NULL THEN ''::text
                    ELSE "left"(COALESCE(r.body, ''::text), 256)
                END, 'created_at', (EXTRACT(epoch FROM r.created_at) * 1000::numeric)::bigint, 'deleted', r.deleted_at IS NOT NULL, 'attachment',
                CASE
                    WHEN r.deleted_at IS NOT NULL THEN NULL::jsonb
                    ELSE COALESCE(( SELECT jsonb_build_object('kind', 'document', 'name', d.name, 'mime', d.mime) AS jsonb_build_object
                       FROM im_message.message_documents d
                      WHERE d.message_id = r.id
                      ORDER BY d.created_at
                     LIMIT 1), ( SELECT jsonb_build_object('kind', 'image', 'mime', i.mime) AS jsonb_build_object
                       FROM im_message.message_images i
                      WHERE i.message_id = r.id
                      ORDER BY i.created_at
                     LIMIT 1))
                END) AS jsonb_build_object
           FROM im_message.messages r
          WHERE r.id = m.reply_to) AS reply_to,
        CASE
            WHEN m.forward_origin_kind IS NULL THEN NULL::jsonb
            ELSE jsonb_build_object('kind', m.forward_origin_kind, 'sender_id', m.forward_origin_sender_id, 'sender_name', COALESCE(m.forward_origin_sender_name, ''::text), 'original_sent_at', (EXTRACT(epoch FROM m.forward_origin_sent_at) * 1000::numeric)::bigint, 'source_message_id', m.forward_from_message_id)
        END AS forward_origin,
    ( SELECT
                CASE
                    WHEN bool_and(st.status = 4) THEN 4
                    ELSE min(st.status) FILTER (WHERE st.status <> 4)::integer
                END AS min
           FROM im_message.message_statuses st
          WHERE st.message_id = m.id) AS delivery_status,
    ( SELECT jsonb_agg(jsonb_build_object('member_id', st.member_id, 'status', st.status, 'delivered_at', st.delivered_at, 'read_at', st.read_at, 'failed_at', st.failed_at, 'via', st.via, 'error', st.error) ORDER BY st.updated_at) AS jsonb_agg
           FROM im_message.message_statuses st
          WHERE st.message_id = m.id) AS statuses,
    ( SELECT jsonb_agg(jsonb_build_object('emoji', e.emoji, 'count', e.cnt, 'reactor_ids', e.reactor_ids, 'last_reacted_at', e.last_ms) ORDER BY e.first_at) AS jsonb_agg
           FROM ( SELECT mr.emoji,
                    count(*)::integer AS cnt,
                    to_jsonb((array_agg(mr.reactor_id ORDER BY mr.created_at))[1:12]) AS reactor_ids,
                    min(mr.created_at) AS first_at,
                    (EXTRACT(epoch FROM max(mr.updated_at)) * 1000::numeric)::bigint AS last_ms
                   FROM im_message.message_reactions mr
                  WHERE mr.message_id = m.id
                  GROUP BY mr.emoji) e) AS reactions,
    (( SELECT count(*)::integer AS count
           FROM im_message.message_revisions rev
          WHERE rev.message_id = m.id)) +
        CASE
            WHEN m.deleted_at IS NOT NULL THEN 1
            ELSE 0
        END AS revision_count,
    m.internal
   FROM im_message.messages m
     LEFT JOIN LATERAL ( SELECT jsonb_build_object('id', td.id, 'role', td.thread_role, 'member_id', m.sender_id) AS member_data
           FROM im_thread.thread_dialog td
          WHERE td.member_id = m.sender_id AND td.thread_id = m.thread_id
          ORDER BY td.id DESC
         LIMIT 1) tm ON true
     LEFT JOIN im_message.buttons_callback bc ON m.type = 5 AND bc.in_reply_to = m.id;
;
