-- +goose Up

-- Reconcile schema drift from in-place edits: "phone" → "phone_number", recreate missing tables.
-- +goose StatementBegin
do $$
begin
    if exists (
        select 1 from information_schema.columns
        where table_schema = 'im_message'
          and table_name = 'message_contacts'
          and column_name = 'phone'
    ) and not exists (
        select 1 from information_schema.columns
        where table_schema = 'im_message'
          and table_name = 'message_contacts'
          and column_name = 'phone_number'
    ) then
        alter table im_message.message_contacts rename column phone to phone_number;
    end if;
end $$;
-- +goose StatementEnd

-- Recreate the tables if the drifted/aborted original never left them behind.
create table if not exists im_message.message_contacts (
    message_id uuid primary key references im_message.messages (id) on delete cascade,
    name text,
    phone_number text,
    email text
);

create table if not exists im_message.message_locations (
    message_id uuid primary key references im_message.messages (id),
    address text,
    name text,
    latitude numeric(10, 7) not null check (latitude >= -90 and latitude <= 90),
    longitude numeric(11, 7) not null check (longitude >= -180 and longitude <= 180)
);

-- Ensure the message type check allows location (6) and contact (7).
alter table im_message.messages drop constraint if exists check_message_type;
alter table im_message.messages
    add constraint check_message_type check (type between 0 and 7) not valid;
alter table im_message.messages validate constraint check_message_type;

-- +goose Down

-- Reconciliation only moves the schema forward to its intended state; there is
-- no meaningful rollback.
select 1;
