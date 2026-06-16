-- +goose Up
ALTER TABLE im_thread.thread
    ADD COLUMN owner_bot_id uuid REFERENCES im_thread.thread_dialog(id);

-- +goose Down
ALTER TABLE im_thread.thread DROP COLUMN owner_bot_id;
