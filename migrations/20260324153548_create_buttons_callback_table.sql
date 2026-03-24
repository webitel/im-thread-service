-- +goose Up
-- +goose StatementBegin
create table if not exists "im_message"."buttons_callback" (
    "message_id" uuid references "im_message"."messages" on delete cascade,
    "button_code" text not null,
    "callback_data" text not null,
    "clicked_at" timestamp with time zone default now(),
    "clicked_by" uuid not null,
    primary key ("message_id", "button_code")
);

create index if not exists "idx_buttons_callback_clicked_at" on "im_message"."buttons_callback" using brin ("clicked_at") with (pages_per_page = 32);

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
drop table if exists "im_message"."buttons_callback";

-- +goose StatementEnd