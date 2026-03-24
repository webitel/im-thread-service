-- +goose Up
-- +goose StatementBegin
create table if not exists "im_message"."message_contacts" (
    "message_id" uuid primary key references "im_message"."messages" ("id") on delete cascade,
    "name" text,
    "email" text,
    "phone_number" text,
    "metadata" jsonb
);

create index if not exists "idx_message_contacts_metadata" on "im_message"."message_contacts" using gin("metadata")
where
    ("metadata" is not null);

alter table
    "im_message"."messages" drop constraint if exists "check_message_type";

alter table
    "im_message"."messages"
add
    constraint "check_message_type" check (
        "type" between 0
        and 7
    ) not valid;

alter table
    "im_message"."messages" validate constraint "check_message_type";

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
alter table
    "im_message"."messages" drop constraint if exists "check_message_type";

update
    "im_message"."messages"
set
    "type" = 0
where
    "type" = 7;

alter table
    "im_message"."messages"
add
    constraint "check_message_type" check(
        "type" between 0
        and 6
    ) not valid;

alter table
    "im_message"."messages" validate constraint "check_message_type";

drop table if exists "im_message"."message_contacts";

-- +goose StatementEnd