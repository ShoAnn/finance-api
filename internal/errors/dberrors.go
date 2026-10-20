package errors

import (
	stderrors "errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Postgres error codes handled explicitly.
const (
	pgUniqueViolation           = "23505"
	pgForeignKeyViolation       = "23503"
	pgCheckViolation            = "23514"
	pgInvalidTextRepresentation = "22P02"
)

// uniqueMessages maps a unique constraint to a client-facing message.
var uniqueMessages = map[string]string{
	"categories_name_key": "category name already exists",
}

// checkDetails maps a check constraint to the field it constrains.
var checkDetails = map[string]Detail{
	"accounts_type_check": {
		Field:   "type",
		Message: "must be one of checking, savings, credit_card, cash, investment",
	},
	"categories_kind_check": {
		Field:   "kind",
		Message: "must be one of income, expense",
	},
}

// FromDB translates a database error into an API error.
//
// Foreign key violations are reported as conflicts, which is correct when
// deleting a resource that other records still reference. Handlers that insert
// a row with a missing reference should report a 400 instead, for example:
//
//	if pgErr := apierr.FromDB(err); pgErr.Code == apierr.CodeConflict {
//		return apierr.InvalidRequest("account does not exist")
//	}
func FromDB(err error) *APIError {
	if err == nil {
		return nil
	}
	if stderrors.Is(err, pgx.ErrNoRows) {
		return NotFound("resource not found")
	}

	var pgErr *pgconn.PgError
	if !stderrors.As(err, &pgErr) {
		return Internal(err)
	}

	switch pgErr.Code {
	case pgUniqueViolation:
		message, ok := uniqueMessages[pgErr.ConstraintName]
		if !ok {
			message = "resource already exists"
		}
		return Conflict(message)
	case pgCheckViolation:
		detail, ok := checkDetails[pgErr.ConstraintName]
		if !ok {
			detail = Detail{Field: "value", Message: "violates a database constraint"}
		}
		return Validation(detail)
	case pgInvalidTextRepresentation:
		return InvalidRequest("malformed value")
	case pgForeignKeyViolation:
		return Conflict("resource is referenced by other records")
	default:
		return Internal(err)
	}
}
