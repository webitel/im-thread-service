-- +goose Up
-- +goose StatementBegin
create table if not exists im_thread.direct_settings (
    "id" uuid default uuidv7() primary key,
    "domain_id" bigint not null,
    "created_at" timestamp with time zone default now() not null,
    "updated_at" timestamp with time zone default now() not null,
    "thread_dialog_id" uuid not null references im_thread.thread_dialog ("id") on delete cascade,
    "title" character varying(255) not null,
    --title can`t be empty with trim to now allow whitespace title
    constraint "title_not_empty" check (trim("title") <> ''),
    unique(domain_id, id)
);
create index if not exists idx_direct_settings_thread_dialog_id on im_thread.direct_settings using btree("thread_dialog_id");
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
drop table if exists im_thread.direct_settings;
-- +goose StatementEnd