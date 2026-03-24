package postgres

import (
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/webitel/webitel-go-kit/pkg/errors"
	"google.golang.org/grpc/codes"
)

const (
	UniqueViolation     string = "23505"
	ForeignKeyViolation string = "23503"
	CheckViolation      string = "23514"
	NotNullViolation    string = "23502"
)

func IsBadRequest(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case UniqueViolation, ForeignKeyViolation, CheckViolation, NotNullViolation:
			return true
		}
	}

	return false
}

func IsConflict(err error) bool {
	var pgErr *pgconn.PgError

	return errors.As(err, &pgErr) && pgErr.Code == UniqueViolation
}

func prepareConflictError(msg string, err error) error {
	return errors.New(
		msg,
		errors.WithCause(err),
		errors.WithCode(codes.AlreadyExists),
	)
}

func prepareBadRequestError(msg string, err error) error {
	return errors.InvalidArgument(msg, errors.WithCause(err))
}
