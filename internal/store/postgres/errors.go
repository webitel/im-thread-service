package postgres

import (
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/webitel/webitel-go-kit/pkg/errors"
	"google.golang.org/grpc/codes"
)

func uniqueViolation(err error) error {
	if pgerr, ok := err.(*pgconn.PgError); ok && pgerr.Code == "23505" {
		return errors.New(
			"conflict: unique violation",
			errors.WithCause(pgerr),
			errors.WithCode(codes.AlreadyExists),
		)
	}
	return nil
}

func foreignKeyViolation(err error) error {
	if pgerr, ok := err.(*pgconn.PgError); ok && pgerr.Code == "23503" {
		return errors.InvalidArgument(
			"conflict: foreign key violation",
			errors.WithCause(pgerr),
		)
	}
	return nil
}
