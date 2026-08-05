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

	err := checkGraphConflicts(g, nil)
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

	err := checkGraphConflicts(g, nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	for _, want := range []string{"Doc.origin", "Doc.source", "Note.author", "3 schema problem(s)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q\ngot: %v", want, err)
		}
	}
}

// uuidKeyedType is newTestType with the one identifier type the base service
// templates can be written against. newTestType's own id is an int64, which is
// exactly the shape the identifier check refuses.
func uuidKeyedType(name string, fields ...*gen.Field) *gen.Type {
	node := newTestType(name, fields...)
	node.ID = newUUIDField("id", nil)
	return node
}

func TestCheckGraphConflicts_NonUUIDIdentifierWithBaseService(t *testing.T) {
	df := ptr(DefaultField())
	g := graphOf(newTestType("Counter", newStringField("label", df))) // int64 id

	err := checkGraphConflicts(g, &ExtensionConfig{GenerateBaseService: true})
	if err == nil {
		t.Fatal("expected an error for a non-UUID primary key with BaseService enabled, got nil")
	}

	// The message is the whole product of the check. A schema author who
	// cannot tell which entity, which type, or what to do next learns nothing
	// that the compile error in their own ent package would not have told them
	// worse.
	for _, want := range []string{"Counter.id", `type "int64"`, "uuid.UUID", "#29", "WithBaseService"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q\ngot: %v", want, err)
		}
	}
}

func TestCheckGraphConflicts_NonUUIDIdentifierIsCheckedForBaseHandlerToo(t *testing.T) {
	df := ptr(DefaultField())
	g := graphOf(newTestType("Counter", newStringField("label", df)))

	// base_handler.tmpl declares uuid.UUID in its own Update signature, so the
	// handler alone is enough to make the identifier type load-bearing.
	if err := checkGraphConflicts(g, &ExtensionConfig{GenerateBaseHandler: true}); err == nil {
		t.Fatal("expected an error with BaseHandler enabled, got nil")
	}
}

func TestCheckGraphConflicts_NonUUIDIdentifierIsFineWithoutBaseService(t *testing.T) {
	df := ptr(DefaultField())
	g := graphOf(newTestType("Counter", newStringField("label", df)))

	// dto.tmpl renders the id through $.ID.Type, so it is correct for any
	// identifier type. Refusing a DTO-only generation would be refusing output
	// that compiles.
	if err := checkGraphConflicts(g, &ExtensionConfig{}); err != nil {
		t.Fatalf("DTO-only generation must not be refused for a non-UUID id, got: %v", err)
	}
}

func TestCheckGraphConflicts_UUIDIdentifierIsAccepted(t *testing.T) {
	df := ptr(DefaultField())
	g := graphOf(uuidKeyedType("Widget", newStringField("label", df)))

	if err := checkGraphConflicts(g, &ExtensionConfig{GenerateBaseService: true, GenerateBaseHandler: true}); err != nil {
		t.Fatalf("expected a uuid.UUID primary key to be accepted, got: %v", err)
	}
}

// TestCheckGraphConflicts_UnannotatedEntityIsNotCheckedForID pins the check to
// the same condition generation uses. An entity with no domain fields produces
// no files at all, so its identifier type cannot make anything fail to compile
// — and refusing it would break every graph that merely contains a join table.
func TestCheckGraphConflicts_UnannotatedEntityIsNotCheckedForID(t *testing.T) {
	g := graphOf(newTestType("Bare", newStringField("label", nil))) // int64 id, no annotation

	if err := checkGraphConflicts(g, &ExtensionConfig{GenerateBaseService: true}); err != nil {
		t.Fatalf("an entity with no domain fields must not be checked, got: %v", err)
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

	if err := checkGraphConflicts(g, nil); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}
