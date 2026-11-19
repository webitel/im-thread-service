-- +goose Up
-- +goose StatementBegin
-- thread is a busy table: do not queue behind a long transaction and block
-- every reader and writer meanwhile; fail fast and let the migration be re-run.
set local lock_timeout = '3s';

-- How the thread's CRM contact was bound: 1 auto (identification),
-- 2 selected, 3 created, 4 searched (operator). Null for bindings made before
-- the source was tracked and for threads without a contact.
alter table "im_thread"."thread"
    add column if not exists "crm_contact_source" smallint;

-- CRM contacts identified for a thread. Rows are only added or extended by new
-- reasons, never removed by identification. reasons holds the kind, trust and
-- origin of each match, never the matched value.
create table if not exists "im_thread"."thread_contact_candidate" (
    "thread_id"      uuid        not null,
    "domain_id"      bigint      not null,
    "crm_contact_id" text        not null,
    "trust"          smallint    not null,
    "reasons"        jsonb       not null default '[]'::jsonb,
    "created_at"     timestamptz not null default now(),
    "updated_at"     timestamptz not null default now(),
    constraint "pk_thread_contact_candidate" primary key ("thread_id", "crm_contact_id"),
    constraint "fk_thread_contact_candidate_thread" foreign key ("thread_id")
        references "im_thread"."thread" ("id") on delete cascade
);

-- Two or more verified candidates on one thread: a hint for a later merge,
-- never merged automatically. contact_ids is sorted, so a set is stored once.
create table if not exists "im_thread"."thread_contact_duplicate" (
    "thread_id"   uuid        not null,
    "domain_id"   bigint      not null,
    "contact_ids" text[]      not null,
    "reasons"     jsonb       not null default '{}'::jsonb,
    "created_at"  timestamptz not null default now(),
    constraint "pk_thread_contact_duplicate" primary key ("thread_id", "contact_ids"),
    constraint "fk_thread_contact_duplicate_thread" foreign key ("thread_id")
        references "im_thread"."thread" ("id") on delete cascade
);

create index if not exists "idx_thread_contact_duplicate_domain_created"
    on "im_thread"."thread_contact_duplicate" ("domain_id", "created_at");

-- Threads whose channel identifiers were already searched, so the first read
-- of the candidate list runs that search once.
create table if not exists "im_thread"."thread_contact_identification" (
    "thread_id"          uuid        not null,
    "domain_id"          bigint      not null,
    "channel_checked_at" timestamptz not null default now(),
    constraint "pk_thread_contact_identification" primary key ("thread_id"),
    constraint "fk_thread_contact_identification_thread" foreign key ("thread_id")
        references "im_thread"."thread" ("id") on delete cascade
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table if exists "im_thread"."thread_contact_identification";
drop table if exists "im_thread"."thread_contact_duplicate";
drop table if exists "im_thread"."thread_contact_candidate";
alter table "im_thread"."thread"
    drop column if exists "crm_contact_source";
-- +goose StatementEnd
