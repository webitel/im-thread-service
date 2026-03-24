-- +goose Up
-- +goose StatementBegin
create table if not exists "im_message"."message_locations" (
    "message_id" uuid primary key references "im_message"."messages" ("id") on delete cascade,
    "address" text,
    "name" text,
    "latitude" numeric(10, 7) not null,
    "longitude" numeric(11, 7) not null
);

alter table
    if exists "im_message"."messages" drop constraint if exists "check_message_type";

alter table
    if exists "im_message"."messages"
add
    constraint "check_message_type" check (
        "type" between 0
        and 6
    ) not valid;

alter table
    if exists "im_message"."messages" validate constraint "check_message_type";

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
    "type" = 6;

alter table
    "im_message"."messages"
add
    constraint if not exists "check_message_type" check (
        "type" between 0
        and 5
    ) not valid;

alter table
    "im_message"."messages" validate constraint "check_message_type";

drop table if exists "im_message"."message_locations";

-- +goose StatementEnd