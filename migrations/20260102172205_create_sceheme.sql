-- +goose Up
-- +goose StatementBegin
create schema im_thread;
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
drop schema im_thread;
-- +goose StatementEnd