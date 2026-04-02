-- +goose Up
-- +goose StatementBegin
create index if not exists "idx_message_thread_id_desc" on "im_message"."messages" using btree("thread_id", "id" desc);

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
drop index if exists "idx_message_thread_id_desc";

-- +goose StatementEnd