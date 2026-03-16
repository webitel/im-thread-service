-- +goose Up
-- +goose StatementBegin
alter table
    "im_message"."messages"
add
    column if not exists "buttons" jsonb;

alter table
    "im_message"."messages" drop constraint if exists "check_message_type";

alter table
    "im_message"."messages"
add
    constraint "check_message_type" check (
        "type" >= 0
        and type <= 5
    );

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
alter table
    "im_message"."messages" drop column if exists "buttons";

update
    "im_message"."messages"
set
    "type" = 0 -- unknown
where
    "type" = 5;

-- interactive
alter table
    "im_message"."messages" drop constraint if exists "check_message_type";

alter table
    "im_message"."messages"
add
    constraint "check_message_type" check (
        "type" >= 0
        and "type" <= 4
    );

-- +goose StatementEnd