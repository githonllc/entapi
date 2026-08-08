package entapi

import (
	"errors"
	"fmt"
	"testing"
)

type fakeSQLStateError struct {
	state   string
	message string
}

func (e *fakeSQLStateError) Error() string    { return e.message }
func (e *fakeSQLStateError) SQLState() string { return e.state }

func TestUniqueViolationSelectsKnownDialects(t *testing.T) {
	tests := []struct {
		dialect string
		ok      bool
	}{
		{dialect: "postgres", ok: true},
		{dialect: "mysql", ok: true},
		{dialect: "sqlite3", ok: true},
		{dialect: "weird", ok: false},
	}
	for _, tc := range tests {
		t.Run(tc.dialect, func(t *testing.T) {
			f, ok := UniqueViolation(tc.dialect)
			if ok != tc.ok {
				t.Fatalf("UniqueViolation(%q) ok = %v, want %v", tc.dialect, ok, tc.ok)
			}
			if (f != nil) != tc.ok {
				t.Fatalf("UniqueViolation(%q) function presence = %v, want %v", tc.dialect, f != nil, tc.ok)
			}
		})
	}
}

func TestUniqueViolationPostgresSQLStateThroughWrapChain(t *testing.T) {
	f, ok := UniqueViolation("postgres")
	if !ok {
		t.Fatal("postgres determination is unavailable")
	}
	err := fmt.Errorf("save: %w", &fakeSQLStateError{state: postgresUniqueSQLState, message: "duplicate key"})
	if !f(err) {
		t.Fatalf("wrapped SQLSTATE %s was not recognised", postgresUniqueSQLState)
	}
}

func TestUniqueViolationPostgresRefusesOtherSQLState(t *testing.T) {
	f, ok := UniqueViolation("postgres")
	if !ok {
		t.Fatal("postgres determination is unavailable")
	}
	if f(&fakeSQLStateError{state: "23503", message: "foreign key violation"}) {
		t.Fatal("SQLSTATE 23503 must not be classified as a uniqueness violation")
	}
}

func TestUniqueViolationPostgresSQLStateIsAuthoritative(t *testing.T) {
	f, ok := UniqueViolation("postgres")
	if !ok {
		t.Fatal("postgres determination is unavailable")
	}
	err := &fakeSQLStateError{
		state:   "P0001",
		message: "RAISE EXCEPTION: " + postgresUniqueViolationMarker,
	}
	if f(err) {
		t.Fatal("a present non-23505 SQLSTATE must prevent fallback to matching text")
	}
}

func TestUniqueViolationTextMarkers(t *testing.T) {
	tests := []struct {
		name     string
		dialect  string
		positive string
		negative string
	}{
		{
			name:     "postgres fallback",
			dialect:  "postgres",
			positive: "pq: duplicate key value " + postgresUniqueViolationMarker + ` "authors_name_key"`,
			negative: "pq: duplicate key value violates exclusion constraint",
		},
		{
			name:     "mysql",
			dialect:  "mysql",
			positive: mysqlUniqueViolationMarker + " (23000): Duplicate entry 'ada' for key 'authors.name'",
			negative: "Error 1452 (23000): Cannot add or update a child row",
		},
		{
			name:     "sqlite",
			dialect:  "sqlite3",
			positive: sqliteUniqueViolationMarker + ": authors.name",
			negative: "FOREIGN KEY constraint failed",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, ok := UniqueViolation(tc.dialect)
			if !ok {
				t.Fatalf("%s determination is unavailable", tc.dialect)
			}
			if !f(fmt.Errorf("save: %w", errors.New(tc.positive))) {
				t.Errorf("%s positive text was not recognised", tc.dialect)
			}
			if f(fmt.Errorf("save: %w", errors.New(tc.negative))) {
				t.Errorf("%s negative text was misclassified", tc.dialect)
			}
		})
	}
}
