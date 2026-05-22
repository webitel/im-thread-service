-- +goose Up
drop index if exists "im_thread"."idx_thread_dialog_domain_thread";

create index if not exists "idx_thread_dialog_domain_thread_not_deleted_at"
on "im_thread"."thread_dialog"
using btree("domain_id", "thread_id")
where "deleted_at" is null;

-- +goose Down
drop index if exists "im_thread"."idx_thread_dialog_domain_thread_not_deleted_at";

create index if not exists "idx_thread_dialog_domain_thread"
on "im_thread"."thread_dialog"
using btree("domain_id", "thread_id");
