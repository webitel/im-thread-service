package postgres

import (
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc/codes"

	"github.com/webitel/webitel-go-kit/pkg/errors"
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
