package entdomain

import (
	"strings"
	"testing"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"
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

// selfRefPair hangs a self-referential edge pair off node, the way gen resolves
// one: the assoc edge, then the inverse edge naming it through Inverse and
// pointing at it through Ref. assocRaw and inverseRaw are the raw DomainEdge
// annotation values, nil meaning the end carries none.
func selfRefPair(node *gen.Type, assocRaw, inverseRaw any) {
	assoc := newEdgeTo("children", node, false, assocRaw)
	inverse := newEdgeTo("parent", node, true, inverseRaw)
	inverse.Inverse = assoc.Name
	inverse.Ref = assoc
	node.Edges = []*gen.Edge{assoc, inverse}
}

func TestCheckGraphConflicts_AsymmetricSelfReferentialEdgePair(t *testing.T) {
	node := newTestType("Tree", newStringField("name", ptr(DefaultField())))
	selfRefPair(node, nil, Edge().InResponse()) // the chained form's result

	err := checkGraphConflicts(graphOf(node), nil)
	if err == nil {
		t.Fatal("expected an error for a self-referential pair annotated on one end only, got nil")
	}

	// The message is the whole product of the check. It has to name both ends,
	// say which one carries what, and spell out the fix — the author is looking
	// at a relation that is simply absent, with no other clue.
	for _, want := range []string{
		"Tree.children", "Tree.parent",
		"no DomainEdge annotation at all",
		"chained form",
		`edge.To("children", Tree.Type)`,
		`edge.From("parent", Tree.Type).Ref("children")`,
		"entdomain.Edge()",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q\ngot: %v", want, err)
		}
	}
}

// TestCheckGraphConflicts_AsymmetricSelfEdgeReportedFromEitherEnd covers the
// mirror image: the assoc end annotated and the inverse forgotten. The chained
// form cannot produce it, so the message must not blame the chained form.
func TestCheckGraphConflicts_AsymmetricSelfEdgeReportedFromEitherEnd(t *testing.T) {
	node := newTestType("Tree", newStringField("name", ptr(DefaultField())))
	selfRefPair(node, Edge().InResponse(), nil)

	err := checkGraphConflicts(graphOf(node), nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if strings.Contains(err.Error(), "chained form") {
		t.Errorf("the chained form cannot annotate the assoc end, so it must not be named as the cause\ngot: %v", err)
	}
	for _, want := range []string{"Tree.children", "Tree.parent", "forgotten"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q\ngot: %v", want, err)
		}
	}
}

// TestCheckGraphConflicts_SymmetricSelfReferentialEdgePairIsAccepted is the
// legal case the fixture internal/fixtures/edges also generates and compiles. A
// guard that rejects it is worse than no guard.
func TestCheckGraphConflicts_SymmetricSelfReferentialEdgePairIsAccepted(t *testing.T) {
	node := newTestType("Category", newStringField("name", ptr(DefaultField())))
	selfRefPair(node, Edge().InResponse(), Edge().InResponse())

	if err := checkGraphConflicts(graphOf(node), nil); err != nil {
		t.Fatalf("a pair annotated on both ends must be accepted, got: %v", err)
	}
}

// TestCheckGraphConflicts_UnannotatedSelfReferentialEdgePairIsAccepted: two bare
// ends are one decision — do not expose the relationship — not a disagreement.
func TestCheckGraphConflicts_UnannotatedSelfReferentialEdgePairIsAccepted(t *testing.T) {
	node := newTestType("Category", newStringField("name", ptr(DefaultField())))
	selfRefPair(node, nil, nil)

	if err := checkGraphConflicts(graphOf(node), nil); err != nil {
		t.Fatalf("a pair with no annotation on either end must be accepted, got: %v", err)
	}
}

// TestCheckGraphConflicts_BareEdgeAnnotationExpressesOneSidedIntent pins the
// escape hatch the refusal message recommends. A bare entdomain.Edge() grants no
// scope, so the generated output is identical to the unannotated end's — the
// only thing it changes is that the decision is on the page.
//
// internal/fixtures/selfrefpartial is the same shape run through real
// generation, which is what proves an empty annotation survives the schema
// load's JSON round-trip rather than marshalling away to nothing.
func TestCheckGraphConflicts_BareEdgeAnnotationExpressesOneSidedIntent(t *testing.T) {
	node := newTestType("Node", newStringField("label", ptr(DefaultField())))
	selfRefPair(node, Edge(), Edge().InResponse())

	if err := checkGraphConflicts(graphOf(node), nil); err != nil {
		t.Fatalf("a bare entdomain.Edge() must express deliberate non-exposure, got: %v", err)
	}
	if hasEdgeScope(node.Edges[0], ScopeResponse) {
		t.Error("a bare entdomain.Edge() must not put the edge in the response")
	}
}

// TestCheckGraphConflicts_CrossEntityPairIsNotChecked is the boundary. The check
// is deliberately confined to pairs whose two ends sit in one Edges() slice: the
// chained declaration can only produce those, and across two entities exposing
// one direction only is the ordinary case, not a symptom.
func TestCheckGraphConflicts_CrossEntityPairIsNotChecked(t *testing.T) {
	df := ptr(DefaultField())
	user := newTestType("User", newStringField("name", df))
	post := newTestType("Post", newStringField("title", df))

	posts := newEdgeTo("posts", post, false, Edge().InResponse())
	posts.Owner = user
	user.Edges = []*gen.Edge{posts}

	author := newEdgeTo("author", user, true, nil)
	author.Inverse = posts.Name
	author.Ref = posts
	author.Owner = post
	post.Edges = []*gen.Edge{author}

	if err := checkGraphConflicts(graphOf(user, post), nil); err != nil {
		t.Fatalf("a cross-entity pair annotated on one side only must not be refused, got: %v", err)
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

// ────────────────────────────────────────────────────────────────────────────
// Query-marker conflicts (#27). Each has the same shape as the immutable one
// above: the annotation asks for a call to something ent never generated.
// ────────────────────────────────────────────────────────────────────────────

// annotatedJSONField returns a named JSON field, for which ent derives no
// order builder and — unless it is Optional — no predicates either.
func annotatedJSONField(name string, optional bool, df *DomainField) *gen.Field {
	f := newField(name, &field.TypeInfo{Type: field.TypeJSON, Ident: "[]string"}, df)
	f.Optional = optional
	return f
}

func TestCheckGraphConflicts_MarkerWithoutQueryScope(t *testing.T) {
	g := graphOf(newTestType("Doc",
		newStringField("token", ptr(InputOnlyField().AsFilterable().AsSortable())),
	))

	err := checkGraphConflicts(g, nil)
	if err == nil {
		t.Fatal("expected an error for a marker on a field withholding ScopeQuery, got nil")
	}
	for _, want := range []string{"Doc.token", "Filterable/Sortable", `scope "query"`, "DefaultField()"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q\ngot: %v", want, err)
		}
	}
}

func TestCheckGraphConflicts_SearchableWithoutContains(t *testing.T) {
	g := graphOf(newTestType("Doc",
		newIntField("count", ptr(DefaultField().AsSearchable())),
	))

	err := checkGraphConflicts(g, nil)
	if err == nil {
		t.Fatal("expected an error for Searchable on a field with no Contains predicate, got nil")
	}
	for _, want := range []string{"Doc.count", "Searchable", "Contains", "CountContains", "AsSearchable()"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q\ngot: %v", want, err)
		}
	}
}

func TestCheckGraphConflicts_FilterableWithNoOperators(t *testing.T) {
	g := graphOf(newTestType("Doc",
		annotatedJSONField("meta", false, ptr(DefaultField().AsFilterable())),
	))

	err := checkGraphConflicts(g, nil)
	if err == nil {
		t.Fatal("expected an error for Filterable on a field with no predicates, got nil")
	}
	for _, want := range []string{"Doc.meta", "Filterable", "no predicates at all", "AsFilterable()"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q\ngot: %v", want, err)
		}
	}
}

func TestCheckGraphConflicts_SortableOnANonComparableField(t *testing.T) {
	// Optional, so the field does have predicates (IsNil/NotNil) — the
	// sortable check must not be reachable only through "has no operators".
	g := graphOf(newTestType("Doc",
		annotatedJSONField("tags", true, ptr(DefaultField().AsSortable())),
	))

	err := checkGraphConflicts(g, nil)
	if err == nil {
		t.Fatal("expected an error for Sortable on a non-comparable field, got nil")
	}
	for _, want := range []string{"Doc.tags", "Sortable", "not comparable", "ByTags", "AsSortable()"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q\ngot: %v", want, err)
		}
	}
}

// TestCheckGraphConflicts_MarkedFieldsThatAgreeWithTheSchema is the converse:
// anything that can be generated correctly is generated, not refused.
func TestCheckGraphConflicts_MarkedFieldsThatAgreeWithTheSchema(t *testing.T) {
	optionalInt := newIntField("score", ptr(DefaultField().AsFilterable()))
	optionalInt.Optional = true

	g := graphOf(newTestType("Doc",
		newStringField("title", ptr(DefaultField().AsFilterable().AsSearchable().AsSortable())),
		optionalInt,
		newTimeField("created_at", ptr(OutputOnlyField().AsFilterable().AsSortable())),
		annotatedJSONField("tags", true, ptr(DefaultField())), // unmarked: nothing to contradict
	))

	if err := checkGraphConflicts(g, nil); err != nil {
		t.Fatalf("expected no conflict, got: %v", err)
	}
}
