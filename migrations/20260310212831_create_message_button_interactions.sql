-- +goose Up
-- +goose StatementBegin
create table if not exists "im_message"."message_button_interactions" (
    "id" uuid primary key default uuidv7(),
    "domain_id" integer not null check ("domain_id" > 0),
    "action" character varying(20) not null check (
        "action" in ('reply', 'postback', 'location', 'contact')
    ),
    "in_reply_to" uuid not null references "im_message"."messages"("id") on delete do nothing,
    "button_code" character varying(255) not null,
    "pressed_by" uuid not null,
    "pressed_at" timestamp with time zone not null default now(),
    unique("pressed_by", "in_reply_to", "action")
);

create index if not exists idx_mbi_message_id on "im_message"."message_button_interactions" using btree("in_reply_to");

create index if not exists idx_mbi_pressed_by on "im_message"."message_button_interactions" using btree("pressed_by");

create table if not exists "im_message"."interaction_postback" (
    "interaction_id" uuid primary key references "im_message"."message_button_interactions"("id"),
    "callback_data" text not null,
);

create table if not exists "im_message"."interaction_contact" (
    "interaction_id" uuid primary key references "im_message"."message_button_interactions"("id"),
    "name" text not null default '',
    "phone_number" text not null default '',
    "metadata" jsonb
);

create table if not exists "im_message"."interaction_location"(
    "interaction_id" uuid primary key references "im_message"."message_button_interactions"("id"),
    "latitude" numeric(10, 7) not null,
    "longitude" numeric(11, 7) not null,
    "city" varchar(100),
    "state" varchar(100),
    "country" varchar(100),
    "postal_code" varchar(20)
);

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
drop table if exists "im_message"."interaction_postback";

drop table if exists "im_message"."interaction_contact";

drop table if exists "im_message"."interaction_location";

drop table if exists "im_message"."message_button_interactions";

-- +goose StatementEnd