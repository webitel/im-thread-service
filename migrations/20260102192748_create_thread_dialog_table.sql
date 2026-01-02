-- +goose Up
-- +goose StatementBegin
create table if not exists im_thread.thread_dialog (
    "id" uuid default uuidv7() primary key,
    "domain_id" bigint not null,
    "created_at" timestamptz default now() not null,
    "updated_at" timestamptz default now() not null,
    "member_id" uuid not null,
    "thread_id" uuid not null,
    "member_of" uuid,
    "direct_to" uuid,
    unique(domain_id, id),
    --[START FK]
    foreign key (domain_id, thread_id) references im_thread.thread(domain_id, id) on delete cascade,
    --[END FK]
    --[START CONSTRAINTS]
    constraint thread_dialog_target_check check ((direct_to is null) <> (member_of is null))
);
create unique index thread_dialog_member_direct_unique on im_thread.thread_dialog(member_id, direct_to)
where direct_to is not null;
create unique index thread_dialog_member_group_unique on im_thread.thread_dialog(member_id, member_of)
where member_of is not null;
create index idx_thread_dialog_domain_thread on im_thread.thread_dialog using btree(domain_id, thread_id);
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
drop index if exists idx_thread_dialog_domain_thread;
drop table if exists im_thread.thread_dialog;
-- +goose StatementEnd