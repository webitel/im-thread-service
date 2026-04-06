-- +goose Up
-- +goose StatementBegin
ALTER TABLE im_thread.thread_dialog
DROP CONSTRAINT IF EXISTS thread_dialog_target_check;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE im_thread.thread_dialog
ADD CONSTRAINT thread_dialog_target_check CHECK ((direct_to is null) <> (member_of is null));
-- +goose StatementEnd
