-- +goose Up
-- +goose StatementBegin
create table if not exists im_thread.thread (
    "id" uuid default uuidv7() primary key,
    "domain_id" bigint not null check (domain_id > 0),
    "created_by" bigint,
    "created_at" timestamptz default now() not null,
    "updated_by" bigint,
    "updated_at" timestamptz default now() not null,
    "kind" smallint not null check (kind in (0, 1, 2)),
    "owner" uuid,
    "subject" varchar(255) default '' not null,
    "description" text default '' not null,
    unique(domain_id, id)
);
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
drop table if exists im_thread.thread;
-- +goose StatementEnd