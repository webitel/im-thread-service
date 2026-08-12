-- +goose Up

-- The reaction API is now declarative and idempotent by construction (a request
-- carries the desired end state: set/replace a non-empty emoji, empty clears),
-- so the send_id dedup ledger is no longer needed. send_id is a pure client echo.
drop table if exists "im_message"."message_reaction_dedup";

-- +goose Down

create table if not exists "im_message"."message_reaction_dedup" (
    "message_id" uuid        not null,
    "reactor_id" uuid        not null,
    "send_id"    text        not null,
    "created_at" timestamptz not null default now(),
    constraint "pk_message_reaction_dedup" primary key ("message_id", "reactor_id", "send_id")
);

create index if not exists "idx_message_reaction_dedup_created_at"
    on "im_message"."message_reaction_dedup" ("created_at");
