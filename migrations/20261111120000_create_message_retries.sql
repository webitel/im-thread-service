-- +goose Up
-- +goose StatementBegin
-- A failed send replaced by its retry (Send* with retry_of). The original is
-- soft-deleted and listed here so history drops it instead of rendering a
-- "deleted" tombstone; retried_by is the message that replaced it.
create table if not exists "im_message"."message_retries" (
    "message_id" uuid        not null,
    "retried_by" uuid        not null,
    "domain_id"  bigint      not null,
    "created_at" timestamptz not null default now(),
    constraint "pk_message_retries" primary key ("message_id"),
    constraint "fk_message_retries_message" foreign key ("message_id")
        references "im_message"."messages" ("id") on delete cascade,
    constraint "fk_message_retries_retried_by" foreign key ("retried_by")
        references "im_message"."messages" ("id") on delete cascade
);

create index if not exists "idx_message_retries_retried_by"
    on "im_message"."message_retries" ("retried_by");
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table if exists "im_message"."message_retries";
-- +goose StatementEnd
