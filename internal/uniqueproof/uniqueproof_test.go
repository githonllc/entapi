package uniqueproof

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"

	entapi "github.com/githonllc/entapi/runtime"
)

const oldLibPQUniqueMessage = `pq: duplicate key value violates unique constraint "authors_email_key"`

func TestPgxSQLStateDetermination(t *testing.T) {
	f, ok := entapi.UniqueViolation("postgres")
	if !ok {
		t.Fatal("postgres uniqueness determination is unavailable")
	}
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "23505 unique violation",
			err:  &pgconn.PgError{Code: "23505", Message: "duplicate key value violates unique constraint"},
			want: true,
		},
		{
			name: "23503 foreign key violation",
			err:  &pgconn.PgError{Code: "23503", Message: "foreign key violation"},
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := fmt.Errorf("save: %w", tc.err)
			if got := f(wrapped); got != tc.want {
				t.Errorf("determination = %v, want %v; err = %v", got, tc.want, wrapped)
			}
		})
	}
}

func TestMySQLUsesDriverTextWithoutSQLStateProbe(t *testing.T) {
	f, ok := entapi.UniqueViolation("mysql")
	if !ok {
		t.Fatal("mysql uniqueness determination is unavailable")
	}
	sqlState23000 := [5]byte{'2', '3', '0', '0', '0'}
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "1062 duplicate entry",
			err:  &mysql.MySQLError{Number: 1062, SQLState: sqlState23000, Message: "Duplicate entry 'ada' for key 'authors.name'"},
			want: true,
		},
		{
			name: "1452 foreign key violation",
			err:  &mysql.MySQLError{Number: 1452, SQLState: sqlState23000, Message: "Cannot add or update a child row"},
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := fmt.Errorf("save: %w", tc.err)
			var stateErr interface{ SQLState() string }
			if errors.As(wrapped, &stateErr) {
				t.Fatalf("MySQLError unexpectedly satisfied the SQLState method probe: %T", stateErr)
			}
			if got := f(wrapped); got != tc.want {
				t.Errorf("text determination = %v, want %v; err = %v", got, tc.want, wrapped)
			}
		})
	}
}

func TestOldLibPQTextShapeFallsBackClosed(t *testing.T) {
	f, ok := entapi.UniqueViolation("postgres")
	if !ok {
		t.Fatal("postgres uniqueness determination is unavailable")
	}
	wrapped := fmt.Errorf("save: %w", errors.New(oldLibPQUniqueMessage))
	if !f(wrapped) {
		t.Fatalf("old lib/pq English text shape was not recognised: %v", wrapped)
	}
}
