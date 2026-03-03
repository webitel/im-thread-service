-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS im_message.message_reads (
    -- Multi-tenancy identifier
    domain_id BIGINT NOT NULL,
    
    -- Foreign key to the thread entity
    thread_id UUID NOT NULL,
    
    -- Foreign key to the specific message entity
    message_id UUID NOT NULL,
    
    -- User/Participant who read the message
    user_id UUID NOT NULL,
    
    -- Timestamp when the message was marked as read
    read_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    -- Composite primary key: ensures uniqueness per user-message pair
    PRIMARY KEY (message_id, user_id),

    -- CONSTRAINT: Ensures the thread exists. 
    -- ON DELETE CASCADE: Automatically removes read records when a thread is deleted.
    CONSTRAINT fk_read_thread 
        FOREIGN KEY (thread_id) 
        REFERENCES im_thread.thread(id) 
        ON DELETE CASCADE,

    -- CONSTRAINT: Ensures the message exists.
    -- ON DELETE CASCADE: Automatically removes read records when a message is deleted.
    CONSTRAINT fk_read_message 
        FOREIGN KEY (message_id) 
        REFERENCES im_message.messages(id) 
        ON DELETE CASCADE
);

-- Index for efficient lookups by thread (e.g., finding all readers in a chat)
CREATE INDEX IF NOT EXISTS idx_message_reads_thread_user 
ON im_message.message_reads(thread_id, user_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS im_message.idx_message_reads_thread_user;
DROP TABLE IF EXISTS im_message.message_reads;
-- +goose StatementEnd