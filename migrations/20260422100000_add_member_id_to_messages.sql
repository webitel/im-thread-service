-- +goose Up
-- +goose StatementBegin

-- add new column
ALTER TABLE im_message.messages
    ADD COLUMN IF NOT EXISTS member_id UUID;

-- fill out the new column with latest thread_dialog_id for each message as sender
UPDATE im_message.messages m
SET member_id = sub.td_id
FROM (
    SELECT DISTINCT ON (m2.id)
        m2.id   AS msg_id,
        td.id   AS td_id
    FROM im_message.messages m2
    JOIN im_thread.thread_dialog td
         ON td.member_id = m2.sender_id
        AND td.thread_id = m2.thread_id
    ORDER BY m2.id, td.id DESC
) sub
WHERE m.id = sub.msg_id;

CREATE INDEX IF NOT EXISTS idx_messages_thread_member_id
    ON im_message.messages (thread_id, member_id, id DESC);


-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS im_message.idx_messages_thread_member_id;

ALTER TABLE im_message.messages
    DROP COLUMN IF EXISTS member_id;

-- +goose StatementEnd
