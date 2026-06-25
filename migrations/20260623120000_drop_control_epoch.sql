-- +goose Up
ALTER TABLE im_thread.thread DROP COLUMN IF EXISTS control_epoch;

-- +goose Down
ALTER TABLE im_thread.thread ADD COLUMN control_epoch bigint NOT NULL DEFAULT 0;
