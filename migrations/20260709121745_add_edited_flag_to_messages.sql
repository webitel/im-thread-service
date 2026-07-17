-- +goose Up
ALTER TABLE "im_message"."messages"
    ADD COLUMN IF NOT EXISTS "edited" boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE "im_message"."messages"
    DROP COLUMN IF EXISTS "edited";
