package entapi

import (
	"errors"
	"strings"
)

const (
	postgresUniqueSQLState = "23505"

	// These markers are pinned verbatim to entgo.io/ent@v0.14.4's
	// dialect/sql/sqlgraph.IsUniqueConstraintError. Keeping the same literals
	// shares text-contract drift with upstream instead of inventing another one.
	mysqlUniqueViolationMarker    = "Error 1062"
	postgresUniqueViolationMarker = "violates unique constraint"
	sqliteUniqueViolationMarker   = "UNIQUE constraint failed"
)

type sqlStateError interface {
	SQLState() string
}

// UniqueViolation returns the uniqueness determination for dialect.
//
// Every determination fails closed: an unrecognised error remains
// unclassified and therefore reaches the HTTP layer as 500. The returned
// function is intended for [ErrorMapper.WithUniqueViolation], where it remains
// gated behind the persistence layer's constraint predicate.
func UniqueViolation(dialect string) (func(error) bool, bool) {
	switch dialect {
	case "postgres":
		return func(err error) bool {
			var stateErr sqlStateError
			if errors.As(err, &stateErr) {
				// A driver's SQLSTATE is authoritative. Falling through on a
				// non-23505 state could turn a user-raised exception whose text
				// happens to match into a false duplicate classification.
				return stateErr.SQLState() == postgresUniqueSQLState
			}
			return strings.Contains(err.Error(), postgresUniqueViolationMarker)
		}, true
	case "mysql":
		return func(err error) bool {
			return strings.Contains(err.Error(), mysqlUniqueViolationMarker)
		}, true
	case "sqlite3":
		return func(err error) bool {
			return strings.Contains(err.Error(), sqliteUniqueViolationMarker)
		}, true
	default:
		return nil, false
	}
}
