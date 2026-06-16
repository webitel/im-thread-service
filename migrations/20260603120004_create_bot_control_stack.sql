-- +goose Up
create table if not exists "im_thread"."bot_control_stack" (
    "id" uuid default uuidv7(),
    "thread_id" uuid     not null references "im_thread"."thread" ("id") on delete cascade,
    -- references thread_dialog; set null if member is removed
    "member_id" uuid     references "im_thread"."thread_dialog" ("id") on delete set null,
    -- 0 = owner bot (never popped), higher = more recent transfer
    "position"  smallint not null,
    constraint "bot_control_stack_pkey" primary key ("id"),
    constraint "bot_control_stack_position_unique" unique ("thread_id", "position")
);

create index if not exists "idx_bot_control_stack_thread"
    on "im_thread"."bot_control_stack" ("thread_id", "position" desc);

-- +goose Down
drop table if exists "im_thread"."bot_control_stack";
