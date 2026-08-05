package entdomain

import (
	"testing"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"
)

func TestDtoImports_BuiltinTypesNeedNothing(t *testing.T) {
	node := newTestType("Widget", newStringField("name", ptr(DefaultField())))

	if got := dtoImports(node); len(got) != 0 {
		t.Errorf("dtoImports() = %v, want none: string needs no import", got)
	}
}

func TestDtoImports_UsesFieldPkgPath(t *testing.T) {
	timeField := newField("created_at",
		&field.TypeInfo{Type: field.TypeTime, Ident: "time.Time", PkgPath: "time"},
		ptr(OutputOnlyField()))
	node := newTestType("Widget", timeField)

	assertImports(t, dtoImports(node), `"time"`)
}

// TestDtoImports_IncludesTheIDField covers the one import that comes from a
// field the template renders without ranging over it.
func TestDtoImports_IncludesTheIDField(t *testing.T) {
	node := newTestType("Widget", newStringField("name", ptr(DefaultField())))
	node.ID = newField("id",
		&field.TypeInfo{Type: field.TypeUUID, Ident: "uuid.UUID", PkgPath: "github.com/google/uuid"},
		nil)

	assertImports(t, dtoImports(node), `"github.com/google/uuid"`)
}

// TestDtoImports_EnumUsesTheEntitySubpackage covers the case ent leaves no
// PkgPath for: an enum without a custom Go type renders as <entitypkg>.<Enum>.
func TestDtoImports_EnumUsesTheEntitySubpackage(t *testing.T) {
	enum := newField("status",
		&field.TypeInfo{Type: field.TypeEnum, Ident: "widget.Status"},
		ptr(DefaultField()))
	node := newTestType("Widget", enum)
	node.Config = &gen.Config{Package: "example.com/app/ent"}

	assertImports(t, dtoImports(node), `"example.com/app/ent/widget"`)
}

// TestDtoImports_EnumWithoutConfigIsSkipped keeps the helper from panicking on
// a hand-built type: no Config means no way to name the subpackage, and
// goimports remains the fallback it always was.
func TestDtoImports_EnumWithoutConfigIsSkipped(t *testing.T) {
	enum := newField("status",
		&field.TypeInfo{Type: field.TypeEnum, Ident: "widget.Status"},
		ptr(DefaultField()))

	if got := dtoImports(newTestType("Widget", enum)); len(got) != 0 {
		t.Errorf("dtoImports() = %v, want none when the node has no Config", got)
	}
}

func TestDtoImports_NilNode(t *testing.T) {
	if got := dtoImports(nil); got != nil {
		t.Errorf("dtoImports(nil) = %v, want nil", got)
	}
}

// TestDtoImports_DeduplicatesAndSorts: two fields of the same type must not
// produce the same import twice, which would not compile.
func TestDtoImports_DeduplicatesAndSorts(t *testing.T) {
	mk := func(name string) *gen.Field {
		return newField(name,
			&field.TypeInfo{Type: field.TypeTime, Ident: "time.Time", PkgPath: "time"},
			ptr(DefaultField()))
	}
	uuidField := newField("owner",
		&field.TypeInfo{Type: field.TypeUUID, Ident: "uuid.UUID", PkgPath: "github.com/google/uuid"},
		ptr(DefaultField()))

	node := newTestType("Widget", mk("created_at"), mk("updated_at"), uuidField)

	assertImports(t, dtoImports(node), `"github.com/google/uuid"`, `"time"`)
}

func TestQuoteImport_AliasesOnlyWhenTheNameDiffers(t *testing.T) {
	cases := []struct {
		pkgName string
		pkgPath string
		want    string
	}{
		{"", "time", `"time"`},
		{"uuid", "github.com/google/uuid", `"github.com/google/uuid"`},
		{"pgtype", "github.com/jackc/pgx/v5/pgtype", `"github.com/jackc/pgx/v5/pgtype"`},
		{"sqlx", "github.com/jmoiron/sql", `sqlx "github.com/jmoiron/sql"`},
	}

	for _, tc := range cases {
		if got := quoteImport(tc.pkgName, tc.pkgPath); got != tc.want {
			t.Errorf("quoteImport(%q, %q) = %q, want %q", tc.pkgName, tc.pkgPath, got, tc.want)
		}
	}
}

func assertImports(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("dtoImports() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("dtoImports()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
