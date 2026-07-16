-- +goose Up
create table if not exists im_message.message_external_ids (
    message_id  uuid not null references im_message.messages ("id") on delete cascade,
    thread_id   uuid not null,
    gate_id     text not null,
    external_id text not null,
    direction   smallint not null default 1,
    created_at  timestamptz not null default now(),
    primary key (gate_id, external_id)
);

create unique index if not exists uq_message_external_ids_message_gate
    on im_message.message_external_ids (message_id, gate_id);

-- +goose Down
drop table if exists im_message.message_external_ids;
