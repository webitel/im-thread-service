-- +goose Up

alter table if exists "im_message"."messages"
drop constraint if exists "check_message_type";

alter table if exists "im_message"."messages"
add constraint if not exists "check_message_type" check ("type" between 0 and 6) not valid;
alter table if exists "im_message"."messages" validate constraint "check_message_type";

create table if not exists "im_message"."message_locations" (
  "message_id" uuid primary key references "im_message"."messages"("id"),
  "address" text,
  "name" text,
  "latitude" numeric(10, 7) not null check (latitude >= '-90'::integer::numeric and latitude <= 90::numeric),
  "longitude" numeric(11, 7) not null check (longitude >= '-180'::integer::numeric and longitude <= 180::numeric)
);

-- +goose Down

update "im_message"."messages"
set "type" = 0
where "type" = 6;

drop table if exists "im_message"."message_locations";

alter table if exists "im_message"."messages"
drop constraint if exists "check_message_type";

alter table if exists "im_message"."messages"
add constraint if not exists "check_message_type" check ("type" between 0 and 5) not valid;
alter table if exists "im_message"."messages" validate constraint "check_message_type";
