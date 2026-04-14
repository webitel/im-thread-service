-- +goose Up
alter table if exists "im_message"."messages" add column if not exists "interactive" jsonb;

alter table if exists "im_message"."messages"
drop constraint if exists "check_message_type";

alter table if exists "im_message"."messages"
add constraint "check_message_type" check ("type" between 0 and 5) not valid;
alter table if exists "im_message"."messages" validate constraint "check_message_type";

create index if not exists "idx_im_message_messages_interactive_attachments"
on "im_message"."messages"
using gin (("interactive"->'attachments') jsonb_path_ops)
where "interactive" is not null;

create table if not exists "im_message"."buttons_callback" (
  "in_reply_to" uuid not null references "im_message"."messages"("id") on delete cascade,
  "button_code" text not null,
  "callback_data" text not null,
  "reacted_at" timestamptz not null default now(),
  "reacted_by" uuid not null,
  primary key ("in_reply_to", "button_code")
);

create index if not exists "idx_im_message_buttons_callback_reacted_at" on "im_message"."buttons_callback" using btree ("reacted_at");

-- +goose Down
update "im_message"."messages" set "type" = 0 where "type" = 5;

drop index if exists "idx_im_message_messages_interactive_attachments";
alter table if exists "im_message"."messages" drop column if exists "interactive";

alter table if exists "im_message"."messages"
drop constraint if exists "check_message_type";

alter table if exists "im_message"."messages"
add constraint if not exists "check_message_type" check ("type" between 0 and 4) not valid;

alter table if exists "im_message"."messages" validate constraint "check_message_type";

drop table if exists "im_message"."buttons_callback";
