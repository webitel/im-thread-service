-- +goose Up
create table if not exists "im_thread"."thread_variables" (
	"thread_id" uuid primary key references "im_thread"."thread"(id) on delete cascade,
	"variables" jsonb not null default '{}'::jsonb
);

-- +goose Down
drop table if exists "im_thread"."thread_variables";
