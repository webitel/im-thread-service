-- +goose Up
-- +goose StatementBegin
-- Display directories of provider gates (source) and call_center queues, refreshed on write.
create table if not exists im_thread.source_ref (
    "gate_id" text primary key,
    "domain_id" bigint not null,
    "name" text not null,
    "type" text not null,
    "deleted" boolean not null default false,
    "updated_at" timestamptz not null default now()
);
create index if not exists idx_source_ref_domain_name
    on im_thread.source_ref (domain_id, name);

create table if not exists im_thread.queue_ref (
    "domain_id" bigint not null,
    "queue_id" bigint not null,
    "name" text not null,
    "deleted" boolean not null default false,
    "updated_at" timestamptz not null default now(),
    primary key (domain_id, queue_id)
);
create index if not exists idx_queue_ref_domain_name
    on im_thread.queue_ref (domain_id, name);

-- Queue through which an agent received the thread; NULL when unknown.
alter table im_thread.thread_dialog add column if not exists queue_id bigint;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
alter table im_thread.thread_dialog drop column if exists queue_id;
drop table if exists im_thread.queue_ref;
drop table if exists im_thread.source_ref;
-- +goose StatementEnd
