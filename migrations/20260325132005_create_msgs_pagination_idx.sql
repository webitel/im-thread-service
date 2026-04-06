-- +goose Up
-- +goose StatementBegin
create index if not exists "idx_messages_pagination" on "im_message"."messages" using btree("domain_id", "thread_id", "id");

drop index if exists "idx_messages_pagination_desc";

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
drop index if exists "idx_messages_pagination";

create index if not exists "idx_messages_pagination_desc" on "im_message"."messages" using btree (created_at DESC, id DESC);

-- +goose StatementEnd