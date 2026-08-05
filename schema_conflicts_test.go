package entdomain

import (
	"strings"
	"testing"

	"entgo.io/ent/entc/gen"
)

// immutableField returns a string field ent has marked Immutable, carrying the
// given annotation.
func immutableField(name string, df *DomainField) *gen.Field {
	f := newStringField(name, df)
	f.Immutable = true
	return f
}

func graphOf(nodes ...*gen.Type) *gen.Graph {
	return &gen.Graph{Nodes: nodes}
}

func TestCheckGraphConflicts_ImmutableFieldWithUpdateScope(t *testing.T) {
	df := ptr(DefaultField()) // grants ScopeUpdate

	g := graphOf(newTestType("Doc",
		newStringField("title", df),
		immutableField("origin", df),
	))

	err := checkGraphConflicts(g)
	if err == nil {
		t.Fatal("expected an error for an Immutable field carrying ScopeUpdate, got nil")
	}

	// The message is the entire product of this check, so assert on it: a
	// schema author who cannot tell which field to change learns nothing.
	for _, want := range []string{"Doc.origin", "Immutable()", `scope "update"`, "SetOrigin", "DocUpdateOne"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q\ngot: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "Doc.title") {
		t.Errorf("mutable field reported as a conflict\ngot: %v", err)
	}
}

func TestCheckGraphConflicts_ReportsEveryConflictAtOnce(t *testing.T) {
	df := ptr(DefaultField())

	g := graphOf(
		newTestType("Doc", immutableField("origin", df), immutableField("source", df)),
		newTestType("Note", immutableField("author", df)),
	)

	err := checkGraphConflicts(g)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	for _, want := range []string{"Doc.origin", "Doc.source", "Note.author", "3 field annotation(s)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q\ngot: %v", want, err)
		}
	}
}

func TestCheckGraphConflicts_Clean(t *testing.T) {
	df := ptr(DefaultField())
	output := ptr(OutputOnlyField())     // no ScopeUpdate
	createOnly := ptr(CreateOnlyField()) // no ScopeUpdate

	g := graphOf(newTestType("Doc",
		newStringField("title", df),          // mutable, ScopeUpdate: fine
		immutableField("created_at", output), // immutable, no ScopeUpdate: fine
		immutableField("origin", createOnly), // immutable, no ScopeUpdate: fine
		immutableField("internal", nil),      // immutable, unannotated: not ours
	))

	if err := checkGraphConflicts(g); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}
