package entdomain

import (
	"strings"
	"testing"

	"entgo.io/ent/entc/gen"
)

func TestDomainFields(t *testing.T) {
	df := ptr(DefaultField())
	node := newTestType("User",
		newStringField("name", df),
		newStringField("bio", nil), // no annotation
		newStringField("email", df),
	)

	got := domainFields(node)
	if len(got) != 2 {
		t.Fatalf("expected 2 domain fields, got %d", len(got))
	}
	if got[0].Name != "name" || got[1].Name != "email" {
		t.Errorf("unexpected fields: %s, %s", got[0].Name, got[1].Name)
	}
}

func TestDomainFieldsEmpty(t *testing.T) {
	node := newTestType("Empty")
	got := domainFields(node)
	if len(got) != 0 {
		t.Fatalf("expected 0 domain fields, got %d", len(got))
	}
}

func TestCreateFields(t *testing.T) {
	withCreate := ptr(DomainFieldWithScopes(ScopeCreate))
	withResponse := ptr(DomainFieldWithScopes(ScopeResponse))

	node := newTestType("User",
		newStringField("name", withCreate),
		newStringField("status", withResponse),
	)

	got := createFields(node)
	if len(got) != 1 {
		t.Fatalf("expected 1 create field, got %d", len(got))
	}
	if got[0].Name != "name" {
		t.Errorf("expected 'name', got %q", got[0].Name)
	}
}

func TestPatchFields(t *testing.T) {
	withUpdate := ptr(DomainFieldWithScopes(ScopeUpdate))
	withCreate := ptr(DomainFieldWithScopes(ScopeCreate))

	node := newTestType("User",
		newStringField("name", withUpdate),
		newStringField("created_by", withCreate),
	)

	got := patchFields(node)
	if len(got) != 1 {
		t.Fatalf("expected 1 patch field, got %d", len(got))
	}
	if got[0].Name != "name" {
		t.Errorf("expected 'name', got %q", got[0].Name)
	}
}

// TestPatchFieldsExcludesImmutableFields pins the half of the rule that comes
// from ent rather than from the annotation. An Immutable() field is absent from
// gen.Type.MutableFields, which is the list ent's own setter template iterates,
// so no Set<Field> exists on the update builder — a patch request carrying one
// would emit a call to a method that was never generated.
//
// checkGraphConflicts refuses this schema before generation, so this asserts on
// a filter that is currently unreachable through the extension. It is here
// because the filter is what makes the emitted code correct, and the refusal is
// what makes the mistake visible; losing either one silently breaks the other.
func TestPatchFieldsExcludesImmutableFields(t *testing.T) {
	withUpdate := ptr(DomainFieldWithScopes(ScopeUpdate))

	frozen := newStringField("origin", withUpdate)
	frozen.Immutable = true

	node := newTestType("User",
		newStringField("name", withUpdate),
		frozen,
	)

	got := patchFields(node)
	if len(got) != 1 {
		t.Fatalf("expected 1 patch field, got %d: %v", len(got), fieldNames(got))
	}
	if got[0].Name != "name" {
		t.Errorf("expected 'name', got %q", got[0].Name)
	}
}

func fieldNames(fields []*gen.Field) []string {
	out := make([]string, len(fields))
	for i, f := range fields {
		out[i] = f.Name
	}
	return out
}

func TestResponseFields(t *testing.T) {
	withResp := ptr(DomainFieldWithScopes(ScopeResponse))
	withCreate := ptr(DomainFieldWithScopes(ScopeCreate))

	node := newTestType("User",
		newStringField("name", withResp),
		newStringField("password", withCreate),
	)

	got := responseFields(node)
	if len(got) != 1 {
		t.Fatalf("expected 1 response field, got %d", len(got))
	}
	if got[0].Name != "name" {
		t.Errorf("expected 'name', got %q", got[0].Name)
	}
}

func TestResponseEdges_NoEdges(t *testing.T) {
	node := newTestType("User", newStringField("name", ptr(DefaultField())))
	got, err := responseEdges(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 response edges, got %d", len(got))
	}
}

// TestResponseEdges_SelectedByAnnotationNotByForeignKey is the regression this
// selector exists for. Every gen.Edge built outside ent's own FK resolution has
// Field() == nil, which is also the shape of a real to-many edge — the foreign
// key lives on the other entity. The previous rule required Field() != nil, so
// a to-many edge was unreachable no matter how it was annotated.
func TestResponseEdges_SelectedByAnnotationNotByForeignKey(t *testing.T) {
	target := newTestType("Post", newStringField("title", ptr(DefaultField())))
	node := newTestType("User", newStringField("name", ptr(DefaultField())))
	node.Edges = []*gen.Edge{
		newEdgeTo("posts", target, false, Edge().InResponse()),
		newEdgeTo("audit", target, false, nil),
	}

	got, err := responseEdges(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Name != "posts" {
		t.Fatalf("response edges = %v, want [posts]", edgeNames(got))
	}
}

// TestResponseEdges_UnannotatedEdgeStaysOutEvenWithAResponseScopedFK pins the
// other half of the split: exposing the scalar and exposing the nested object
// are independent decisions.
func TestResponseEdges_UnannotatedEdgeStaysOutEvenWithAResponseScopedFK(t *testing.T) {
	target := newTestType("User", newStringField("name", ptr(DefaultField())))
	node := newTestType("Post",
		newStringField("title", ptr(DefaultField())),
		newUUIDField("reviewer_id", ptr(DefaultField())), // response-scoped scalar
	)
	node.Edges = []*gen.Edge{newEdgeTo("reviewer", target, true, nil)}

	got, err := responseEdges(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("response edges = %v, want none", edgeNames(got))
	}
}

// TestResponseEdges_TargetWithoutDomainFieldsIsAnError keeps the generator
// dependency-closed. A target with no DomainField annotation is skipped
// wholesale by the generator, so no <Target>Summary exists; dropping the edge
// would silently narrow the response, and emitting the reference would surface
// as an undefined symbol in the consumer's build.
func TestResponseEdges_TargetWithoutDomainFieldsIsAnError(t *testing.T) {
	target := newTestType("Organization") // no annotated fields
	node := newTestType("User", newStringField("name", ptr(DefaultField())))
	node.Edges = []*gen.Edge{newEdgeTo("org", target, true, Edge().InResponse())}

	got, err := responseEdges(node)
	if err == nil {
		t.Fatalf("expected an error, got edges %v", edgeNames(got))
	}
	for _, want := range []string{"User", "org", "Organization"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
}

// newEdgeTo builds a gen.Edge pointing at target. Field() is left nil, which is
// what ent produces for every edge whose foreign key is on the other side.
func newEdgeTo(name string, target *gen.Type, unique bool, raw any) *gen.Edge {
	e := &gen.Edge{Name: name, Type: target, Unique: unique}
	if raw != nil {
		e.Annotations = gen.Annotations{"DomainEdge": raw}
	}
	return e
}

func edgeNames(edges []*gen.Edge) []string {
	names := make([]string, len(edges))
	for i, e := range edges {
		names[i] = e.Name
	}
	return names
}
