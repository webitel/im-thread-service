-- +goose Up
-- +goose StatementBegin
ALTER TABLE im_thread.thread_dialog
    ADD COLUMN IF NOT EXISTS invited_by UUID,
    ADD COLUMN IF NOT EXISTS leave_reason TEXT;

ALTER TABLE im_thread.thread_dialog
    ADD CONSTRAINT thread_dialog_invited_by_fkey
    FOREIGN KEY (invited_by)
    REFERENCES im_thread.thread_dialog(id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE im_thread.thread_dialog
    DROP CONSTRAINT IF EXISTS thread_dialog_invited_by_fkey;

ALTER TABLE im_thread.thread_dialog
    DROP COLUMN IF EXISTS leave_reason,
    DROP COLUMN IF EXISTS invited_by;
-- +goose StatementEnd
