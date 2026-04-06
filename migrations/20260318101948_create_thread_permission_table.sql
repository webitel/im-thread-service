-- +goose Up
-- +goose StatementBegin
CREATE TABLE im_thread.thread_permission (
    id UUID DEFAULT uuidv7() PRIMARY KEY,
    thread_id UUID NOT NULL,
    thread_dialog_id UUID NOT NULL,

    can_send_messages BOOLEAN NOT NULL DEFAULT TRUE,
    can_add_members BOOLEAN NOT NULL DEFAULT TRUE,
    can_remove_members BOOLEAN NOT NULL DEFAULT FALSE,
    can_change_members_permissions BOOLEAN NOT NULL DEFAULT FALSE,
    can_change_thread_info BOOLEAN NOT NULL DEFAULT FALSE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (thread_id) REFERENCES im_thread.thread(id) ON DELETE CASCADE,
    FOREIGN KEY (thread_dialog_id) REFERENCES im_thread.thread_dialog(id) ON DELETE CASCADE
);

CREATE INDEX idx_thread_permission_member_id
    ON im_thread.thread_permission(thread_dialog_id);


ALTER TABLE im_thread.thread_dialog 
ADD COLUMN thread_role INT NOT NULL DEFAULT 0;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE im_thread.thread_permission CASCADE;

ALTER TABLE im_thread.thread_dialog 
DROP COLUMN thread_role;
-- +goose StatementEnd
