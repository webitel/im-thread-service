-- +goose Up
-- +goose StatementBegin
alter table
    if exists "im_message"."messages"
add
    column if not exists "interactive" jsonb;

alter table
    if exists "im_message"."messages" drop constraint if exists "check_message_type";

alter table
    if exists "im_message"."messages"
add
    constraint if not exists "check_message_type" check (
        "type" between 0
        and 5
    ) not valid;

alter table
    if exists "im_message"."messages" validate constraint "check_message_type";

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
alter table
    if exists "im_message"."messages" drop constraint if exists "check_message_type";

update
    "im_message"."messages"
set
    "type" = 0
where
    "type" = 5;

alter table
    if exists "im_message"."messages"
add
    constraint if not exists "check_message_type" check (
        "type" between 0
        and 4
    ) not valid;

alter table
    if exists "im_message"."messages" validate constraint "check_message_type";

alter table
    if exists "im_message"."messages" drop column if exists "interactive";

-- +goose StatementEnd