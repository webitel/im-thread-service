-- +goose Up
-- Webitel CRM contact (contacts.contact.id) an operator selected for the thread.
alter table "im_thread"."thread"
    add column if not exists "crm_contact_id" text;

-- +goose Down
alter table "im_thread"."thread"
    drop column if exists "crm_contact_id";
