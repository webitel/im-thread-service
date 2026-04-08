-- +goose Up
create table if not exists "im_message"."message_contacts" (
  "message_id" uuid primary key references "im_message"."messages"("id") on delete cascade,
  "name" text,
  "phone_number" text,
  "email" text
);

alter table if exists "im_message"."messages"
drop constraint if exists "check_message_type";

alter table if exists "im_message"."messages"
add constraint "check_message_type" check ("type" between 0 and 7) not valid;
alter table if exists "im_message"."messages" validate constraint "check_message_type";

-- +goose Down
update "im_message"."messages" set "type" = 0 where "type" = 7;
alter table if exists "im_message"."messages"
drop constraint if exists "check_message_type";

alter table if exists "im_message"."messages"
add constraint if not exists "check_message_type" check ("type" between 0 and 6) not valid;

alter table if exists "im_message"."messages" validate constraint "check_message_type";

drop table if exists "im_message"."message_contacts";
