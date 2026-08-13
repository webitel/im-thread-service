-- +goose Up

-- message_reads (20260303) is dead: 0 rows, no code references, not read by any
-- view. Read state is now the thread_dialog.last_read_message_id watermark.
drop table if exists "im_message"."message_reads";

-- +goose Down

create table if not exists "im_message"."message_reads" (
    "domain_id"  bigint      not null,
    "thread_id"  uuid        not null,
    "message_id" uuid        not null,
    "user_id"    uuid        not null,
    "read_at"    timestamptz not null default now(),
    constraint "pk_message_reads" primary key ("message_id", "user_id")
);
