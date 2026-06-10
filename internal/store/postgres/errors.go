package postgres

import (
	"cmp"

	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc/codes"

	"github.com/webitel/webitel-go-kit/pkg/errors"
)

func uniqueViolation(err error, errMessage ...string) error {
	finalErrorMessage := "conflict: unique violation"
	if len(errMessage) != 0 {
		finalErrorMessage = cmp.Or(errMessage[0], finalErrorMessage)
	}

	if pgerr, ok := err.(*pgconn.PgError); ok && pgerr.Code == "23505" {
		return errors.New(
			finalErrorMessage,
			errors.WithCause(pgerr),
			errors.WithCode(codes.AlreadyExists),
		)
	}

	return nil
}
