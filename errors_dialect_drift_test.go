package entapi

import (
	"testing"

	"entgo.io/ent/dialect"

	entapirt "github.com/githonllc/entapi/runtime"
)

func TestEntDialectNamesHaveUniqueViolationDeterminations(t *testing.T) {
	for _, name := range []string{dialect.SQLite, dialect.Postgres, dialect.MySQL} {
		if _, ok := entapirt.UniqueViolation(name); !ok {
			t.Errorf("ent dialect %q has no uniqueness determination", name)
		}
	}
}
