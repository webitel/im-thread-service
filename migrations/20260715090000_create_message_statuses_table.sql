-- +goose Up
-- +goose StatementBegin
-- Per-recipient delivery state of a message:
-- 1=sent 2=delivered 3=read 4=failed.
-- Rows are created with SENT for every thread member except the sender
-- within the message-save transaction; later transitions are monotonic
-- upserts driven by delivery confirmations (stream ACK, push, provider
-- webhook receipts, bot dispatch).
CREATE TABLE IF NOT EXISTS im_message.message_statuses (
    -- Multi-tenancy identifier
    domain_id BIGINT NOT NULL,

    -- Foreign key to the thread entity
    thread_id UUID NOT NULL,

    -- Foreign key to the specific message entity
    message_id UUID NOT NULL,

    -- Recipient of the message (thread_dialog.member_id / contact id)
    member_id UUID NOT NULL,

    -- Delivery state: 1=sent 2=delivered 3=read 4=failed
    status SMALLINT NOT NULL DEFAULT 1
        CONSTRAINT check_message_status CHECK (status BETWEEN 1 AND 4),

    -- Timestamps of reaching each state (kept once set)
    delivered_at TIMESTAMPTZ,
    read_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ,

    -- Provider error code/detail for FAILED
    error JSONB,

    -- Confirmation source: ws|push|provider|bot
    via TEXT,

    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- One status row per recipient-message pair
    PRIMARY KEY (message_id, member_id),

    CONSTRAINT fk_message_statuses_thread
        FOREIGN KEY (thread_id)
        REFERENCES im_thread.thread(id)
        ON DELETE CASCADE,

    CONSTRAINT fk_message_statuses_message
        FOREIGN KEY (message_id)
        REFERENCES im_message.messages(id)
        ON DELETE CASCADE
);

-- Lookups by thread member (unread counters, read-up-to bulk updates)
CREATE INDEX IF NOT EXISTS idx_message_statuses_thread
    ON im_message.message_statuses (thread_id, member_id, status);

-- Backfill read receipts from im_message.message_reads, which is absorbed
-- by message_statuses (read = status 3 + read_at). The legacy table is kept
-- for now but is no longer written to.
INSERT INTO im_message.message_statuses
    (domain_id, thread_id, message_id, member_id, status, read_at, updated_at)
SELECT
    r.domain_id,
    r.thread_id,
    r.message_id,
    r.user_id,
    3,
    r.read_at,
    COALESCE(r.read_at, now())
FROM im_message.message_reads r
ON CONFLICT (message_id, member_id) DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS im_message.idx_message_statuses_thread;
DROP TABLE IF EXISTS im_message.message_statuses;
-- +goose StatementEnd
