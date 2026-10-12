-- +goose Up
-- +goose StatementBegin
create table if not exists im_thread.thread_preview (
    "id" uuid default uuidv7() primary key,
    "domain_id" bigint not null,
    "thread_id" uuid not null,
    "contact_id" uuid not null,
    "initiator_id" uuid not null,
    "created_at" timestamptz not null default now(),
    "expires_at" timestamptz not null,
    "revoked_at" timestamptz,
    "revoke_reason" text,
    foreign key (domain_id, thread_id) references im_thread.thread(domain_id, id) on delete cascade,
    constraint thread_preview_revoke_reason_check check (revoke_reason in ('remove', 'upgrade', 'expired')),
    constraint thread_preview_revoked_consistency_check check ((revoked_at is null) = (revoke_reason is null)),
    constraint thread_preview_expires_after_created_check check (expires_at > created_at)
);
-- At most one unrevoked preview per (thread, contact): the ON CONFLICT arbiter of Upsert, and the
-- lookup for every "active preview of contact X in thread Y" check (GetActiveForUpdate, CanRead, read ACLs).
-- Expired rows are closed with revoke_reason = 'expired' before a new grant, since now() can't be in the predicate.
create unique index if not exists idx_thread_preview_thread_contact_unrevoked
    on im_thread.thread_preview (thread_id, contact_id) where revoked_at is null;
-- Backs SearchVariables' "threads readable by the caller" subquery.
create index if not exists idx_thread_preview_contact_unrevoked
    on im_thread.thread_preview (contact_id) where revoked_at is null;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table if exists im_thread.thread_preview;
-- +goose StatementEnd
