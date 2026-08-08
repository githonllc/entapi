package entapi

import (
	"strings"
	"testing"

	"entgo.io/ent/entc/gen"
)

// newSoftDeletableType builds a node carrying the marker the way real
// generation sees it. raw is what sits in the annotation map: during codegen
// that is a DomainSoftDelete value, but a schema loaded from its serialized
// form hands over a map[string]interface{} instead, and both have to work.
func newSoftDeletableType(name string, raw any, fields ...*gen.Field) *gen.Type {
	node := newTestType(name, fields...)
	node.Annotations = gen.Annotations{SoftDeleteAnnotationName: raw}
	return node
}

func TestIsSoftDeletable(t *testing.T) {
	deletedAt := newTimeField(SoftDeleteField, nil)
	deletedAt.Optional = true
	deletedAt.Nillable = true

	cases := []struct {
		name string
		node *gen.Type
		want bool
	}{
		{
			name: "no annotation at all",
			node: newTestType("Plain", deletedAt),
			want: false,
		},
		{
			// The whole point of retiring the convention: the column alone
			// decides nothing.
			name: "deleted_at field but no mixin",
			node: newTestType("Ledger", deletedAt),
			want: false,
		},
		{
			name: "annotation as a concrete value (codegen path)",
			node: newSoftDeletableType("Doc", DomainSoftDelete{Field: SoftDeleteField}, deletedAt),
			want: true,
		},
		{
			name: "annotation as a pointer",
			node: newSoftDeletableType("Doc", &DomainSoftDelete{Field: SoftDeleteField}, deletedAt),
			want: true,
		},
		{
			// This is the shape ent actually delivers: entc/load marshals the
			// schema to JSON, so the annotation arrives as a map.
			name: "annotation as a map (loaded-schema path)",
			node: newSoftDeletableType("Doc", map[string]any{"field": SoftDeleteField}, deletedAt),
			want: true,
		},
		{
			name: "annotation key present but nil",
			node: newSoftDeletableType("Doc", nil, deletedAt),
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSoftDeletable(tc.node); got != tc.want {
				t.Errorf("isSoftDeletable(%s) = %v, want %v", tc.node.Name, got, tc.want)
			}
		})
	}

	if isSoftDeletable(nil) {
		t.Error("isSoftDeletable(nil) = true, want false")
	}
}

func TestSoftDeleteFieldResolvesThroughTheAnnotation(t *testing.T) {
	deletedAt := newTimeField(SoftDeleteField, nil)
	deletedAt.Optional = true
	other := newStringField("title", nil)

	node := newSoftDeletableType("Doc", map[string]any{"field": SoftDeleteField}, other, deletedAt)
	got := softDeleteField(node)
	if got == nil {
		t.Fatalf("softDeleteField returned nil for a node carrying the marker")
	}
	if got.Name != SoftDeleteField {
		t.Errorf("softDeleteField = %q, want %q", got.Name, SoftDeleteField)
	}

	// The name comes from the annotation, not from a constant in the template:
	// a marker naming something else resolves to that something else.
	renamed := newSoftDeletableType("Doc", map[string]any{"field": "title"}, other, deletedAt)
	if got := softDeleteField(renamed); got == nil || got.Name != "title" {
		t.Errorf("softDeleteField followed the constant instead of the annotation: got %v", got)
	}

	// A marker naming a field the entity does not have resolves to nil, which
	// is what checkGraphConflicts refuses on.
	missing := newSoftDeletableType("Doc", map[string]any{"field": "nope"}, other)
	if got := softDeleteField(missing); got != nil {
		t.Errorf("softDeleteField = %v for a marker naming an absent field, want nil", got)
	}

	if softDeleteField(newTestType("Plain")) != nil {
		t.Error("softDeleteField on an unmarked node returned non-nil")
	}
}

func TestSoftDeleteTypesAndImports(t *testing.T) {
	deletedAt := newTimeField(SoftDeleteField, nil)
	deletedAt.Optional = true

	marked := newSoftDeletableType("Doc", map[string]any{"field": SoftDeleteField}, deletedAt)
	plain := newTestType("Note", newStringField("body", nil))

	g := &gen.Graph{
		Config: &gen.Config{Package: "example.com/project/ent"},
		Nodes:  []*gen.Type{plain, marked},
	}

	types := softDeleteTypes(g)
	if len(types) != 1 || types[0].Name != "Doc" {
		t.Fatalf("softDeleteTypes = %v, want [Doc]", types)
	}

	imports := softDeleteImports(g)
	want := `"example.com/project/ent/doc"`
	if len(imports) != 1 || imports[0] != want {
		t.Errorf("softDeleteImports = %v, want [%s]", imports, want)
	}

	// An empty graph is the case that decides whether the file is written at
	// all, so it is not a degenerate input here.
	empty := &gen.Graph{Config: &gen.Config{Package: "example.com/project/ent"}, Nodes: []*gen.Type{plain}}
	if got := softDeleteTypes(empty); len(got) != 0 {
		t.Errorf("softDeleteTypes on a graph with no mixin = %v, want none", got)
	}
	if got := softDeleteImports(empty); len(got) != 0 {
		t.Errorf("softDeleteImports on a graph with no mixin = %v, want none", got)
	}
	if softDeleteTypes(nil) != nil || softDeleteImports(nil) != nil {
		t.Error("the graph-level helpers do not tolerate a nil graph")
	}
}

// TestSoftDeleteMixinDeclaresNoHooks pins the design decision, not an
// implementation detail. A mixin carrying hooks or interceptors switches ent's
// runtime generation to the format that obliges every consumer to empty-import
// <project>/ent/runtime — so adopting soft delete would change how the
// consumer's whole project generates. Adding one here would break that
// silently, in their repository rather than in this one.
func TestSoftDeleteMixinDeclaresNoHooks(t *testing.T) {
	m := SoftDeleteMixin{}
	if got := m.Hooks(); len(got) != 0 {
		t.Errorf("SoftDeleteMixin.Hooks() = %d hooks, want 0; see the README section on the empty import", len(got))
	}
	if got := m.Interceptors(); len(got) != 0 {
		t.Errorf("SoftDeleteMixin.Interceptors() = %d interceptors, want 0; see the README section on the empty import", len(got))
	}
	if m.Policy() != nil {
		t.Error("SoftDeleteMixin.Policy() is non-nil, which has the same consequence as a hook")
	}

	fields := m.Fields()
	if len(fields) != 1 {
		t.Fatalf("SoftDeleteMixin.Fields() = %d fields, want exactly the tombstone", len(fields))
	}
	d := fields[0].Descriptor()
	if d.Name != SoftDeleteField {
		t.Errorf("tombstone field is named %q, want %q", d.Name, SoftDeleteField)
	}
	// Optional is what makes ent generate <Field>IsNil, which the traverser is
	// built from; Nillable is what lets "not deleted" load as nil.
	if !d.Optional {
		t.Error("the tombstone field is not Optional, so ent generates no DeletedAtIsNil predicate")
	}
	if !d.Nillable {
		t.Error("the tombstone field is not Nillable, so a loaded entity cannot report 'not deleted' as nil")
	}

	ann := m.Annotations()
	if len(ann) != 1 || ann[0].Name() != SoftDeleteAnnotationName {
		t.Fatalf("SoftDeleteMixin.Annotations() = %v, want exactly the %s marker", ann, SoftDeleteAnnotationName)
	}
	sd, ok := ann[0].(DomainSoftDelete)
	if !ok {
		t.Fatalf("the marker is a %T, want DomainSoftDelete", ann[0])
	}
	if sd.Field != SoftDeleteField {
		t.Errorf("the marker names field %q, want %q — the generator resolves the column through it", sd.Field, SoftDeleteField)
	}
}

// TestUnusableSoftDeleteFieldIsRefused covers the two ways the marker can name
// something the traverser cannot be written against. Neither is reachable by
// embedding SoftDeleteMixin — both mean the annotation was attached by hand —
// but the failure they prevent is a compile error inside the consumer's own ent
// package, naming a predicate they never wrote.
func TestUnusableSoftDeleteFieldIsRefused(t *testing.T) {
	title := newStringField("title", ptr(DefaultField()))

	t.Run("marker names an absent field", func(t *testing.T) {
		node := newSoftDeletableType("Doc", map[string]any{"field": "nope"}, title)
		node.ID = newUUIDField("id", nil)
		g := &gen.Graph{Config: &gen.Config{Package: "example.com/project/ent"}, Nodes: []*gen.Type{node}}

		err := checkGraphConflicts(g)
		if err == nil {
			t.Fatal("generation was not refused")
		}
		for _, want := range []string{"Doc", SoftDeleteAnnotationName, `"nope"`, "SoftDeleteMixin"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("refusal does not mention %q.\ngot: %v", want, err)
			}
		}
	})

	t.Run("tombstone field is not Optional", func(t *testing.T) {
		deletedAt := newTimeField(SoftDeleteField, nil)
		deletedAt.Nillable = true // Nillable without Optional: no IsNil predicate.
		node := newSoftDeletableType("Doc", map[string]any{"field": SoftDeleteField}, title, deletedAt)
		node.ID = newUUIDField("id", nil)
		g := &gen.Graph{Config: &gen.Config{Package: "example.com/project/ent"}, Nodes: []*gen.Type{node}}

		err := checkGraphConflicts(g)
		if err == nil {
			t.Fatal("generation was not refused")
		}
		for _, want := range []string{"Doc.deleted_at", "Optional", "DeletedAtIsNil", "SoftDeleteMixin"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("refusal does not mention %q.\ngot: %v", want, err)
			}
		}
	})

	t.Run("a well-formed marker is not refused", func(t *testing.T) {
		deletedAt := newTimeField(SoftDeleteField, nil)
		deletedAt.Optional = true
		deletedAt.Nillable = true
		node := newSoftDeletableType("Doc", map[string]any{"field": SoftDeleteField}, title, deletedAt)
		node.ID = newUUIDField("id", nil)
		g := &gen.Graph{Config: &gen.Config{Package: "example.com/project/ent"}, Nodes: []*gen.Type{node}}

		if err := checkGraphConflicts(g); err != nil {
			t.Fatalf("a conforming schema was refused: %v", err)
		}
	})

	t.Run("an entity with no domain fields is still checked", func(t *testing.T) {
		// The per-type generators skip such a node, but soft delete is a
		// property of the ent schema rather than of the HTTP surface, so the
		// check must run before that gate.
		node := newSoftDeletableType("Doc", map[string]any{"field": "nope"})
		node.ID = newUUIDField("id", nil)
		g := &gen.Graph{Config: &gen.Config{Package: "example.com/project/ent"}, Nodes: []*gen.Type{node}}

		if err := checkGraphConflicts(g); err == nil {
			t.Fatal("the marker check was skipped for an entity with no domain fields")
		}
	})
}
