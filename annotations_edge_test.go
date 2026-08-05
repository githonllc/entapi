package entdomain

import (
	"testing"

	"entgo.io/ent/entc/gen"
)

// newEdge builds a gen.Edge carrying the given raw annotation value. The value
// is deliberately untyped: the whole point of these tests is that the reader
// must cope with more than one representation of the same annotation.
func newEdge(name string, raw any) *gen.Edge {
	e := &gen.Edge{Name: name}
	if raw != nil {
		e.Annotations = gen.Annotations{"DomainEdge": raw}
	}
	return e
}

// realLoadedMap is the exact value ent hands to a generator for
//
//	edge.To("posts", Post.Type).Annotations(entdomain.Edge().InResponse())
//
// captured from entc.LoadGraph against internal/fixture/ent/schema. Keeping the
// literal here rather than a hand-guessed shape is the point: the map branch of
// getDomainEdgeAnnotation exists for this value and no other.
func realLoadedMap() map[string]interface{} {
	return map[string]interface{}{
		"scopes": []interface{}{"response"},
	}
}

func TestGetDomainEdgeAnnotation_MapRepresentation(t *testing.T) {
	t.Run("the shape ent actually produces", func(t *testing.T) {
		got := getDomainEdgeAnnotation(newEdge("posts", realLoadedMap()))
		if got == nil {
			t.Fatal("map representation was dropped; generation would see no annotation at all")
		}
		if len(got.Scopes) != 1 || got.Scopes[0] != ScopeResponse {
			t.Fatalf("scopes = %#v, want [%s]", got.Scopes, ScopeResponse)
		}
		if got.JSONKey != "" {
			t.Errorf("json_key = %q, want empty", got.JSONKey)
		}
	})

	t.Run("json_key survives the round-trip", func(t *testing.T) {
		m := realLoadedMap()
		m["json_key"] = "written_by"
		got := getDomainEdgeAnnotation(newEdge("author", m))
		if got == nil {
			t.Fatal("nil annotation")
		}
		if got.JSONKey != "written_by" {
			t.Fatalf("json_key = %q, want %q", got.JSONKey, "written_by")
		}
	})

	t.Run("multiple scopes survive", func(t *testing.T) {
		m := map[string]interface{}{
			"scopes": []interface{}{"response", "query"},
		}
		got := getDomainEdgeAnnotation(newEdge("posts", m))
		if got == nil {
			t.Fatal("nil annotation")
		}
		if len(got.Scopes) != 2 || got.Scopes[0] != ScopeResponse || got.Scopes[1] != ScopeQuery {
			t.Fatalf("scopes = %#v, want [response query]", got.Scopes)
		}
	})

	t.Run("empty map yields an annotation with no scopes, not nil", func(t *testing.T) {
		got := getDomainEdgeAnnotation(newEdge("posts", map[string]interface{}{}))
		if got == nil {
			t.Fatal("an empty annotation is still an annotation")
		}
		if len(got.Scopes) != 0 {
			t.Fatalf("scopes = %#v, want none", got.Scopes)
		}
	})

	t.Run("a map that cannot decode yields nil rather than a panic", func(t *testing.T) {
		m := map[string]interface{}{"scopes": "response"} // string, not a list
		if got := getDomainEdgeAnnotation(newEdge("posts", m)); got != nil {
			t.Fatalf("want nil for an undecodable annotation, got %#v", got)
		}
	})
}

// TestHasEdgeScope_MapRepresentation exercises the selector callers actually
// use, not just the reader underneath it. A reader that decodes correctly but a
// selector that consults the wrong field is still a silently missing edge.
func TestHasEdgeScope_MapRepresentation(t *testing.T) {
	e := newEdge("posts", realLoadedMap())
	if !hasEdgeScope(e, ScopeResponse) {
		t.Error("response scope not seen through the map representation")
	}
	if hasEdgeScope(e, ScopeCreate) {
		t.Error("create scope reported for an annotation that does not carry it")
	}
}

func TestResponseEdgeSet_MapRepresentation(t *testing.T) {
	node := &gen.Type{
		Name: "User",
		Edges: []*gen.Edge{
			newEdge("posts", realLoadedMap()),                                   // to-many, annotated
			newEdge("secrets", map[string]interface{}{"scopes": []any{}}),       // annotated, no scope
			newEdge("audit", nil),                                               // unannotated
			newEdge("owner", map[string]interface{}{"scopes": []any{"create"}}), // wrong scope
		},
	}
	got := responseEdgeSet(node)
	if len(got) != 1 || got[0].Name != "posts" {
		names := make([]string, len(got))
		for i, e := range got {
			names[i] = e.Name
		}
		t.Fatalf("response edges = %v, want [posts]", names)
	}
}

func TestEdgeJSONKey_MapRepresentation(t *testing.T) {
	t.Run("falls back to the edge name", func(t *testing.T) {
		if got := edgeJSONKey(newEdge("posts", realLoadedMap())); got != "posts" {
			t.Fatalf("json key = %q, want %q", got, "posts")
		}
	})

	t.Run("override wins", func(t *testing.T) {
		m := realLoadedMap()
		m["json_key"] = "articles"
		if got := edgeJSONKey(newEdge("posts", m)); got != "articles" {
			t.Fatalf("json key = %q, want %q", got, "articles")
		}
	})

	t.Run("unannotated edge falls back to the edge name", func(t *testing.T) {
		if got := edgeJSONKey(newEdge("posts", nil)); got != "posts" {
			t.Fatalf("json key = %q, want %q", got, "posts")
		}
	})
}

// TestGetDomainEdgeAnnotation_GoTypedRepresentation covers the other half of
// the contract: during in-process codegen the annotation is still the Go value
// the schema author wrote.
func TestGetDomainEdgeAnnotation_GoTypedRepresentation(t *testing.T) {
	want := Edge().InResponse().As("written_by")

	t.Run("pointer", func(t *testing.T) {
		got := getDomainEdgeAnnotation(newEdge("author", &want))
		if got == nil || got.JSONKey != "written_by" || !got.hasScope(ScopeResponse) {
			t.Fatalf("pointer representation not read: %#v", got)
		}
	})

	t.Run("value", func(t *testing.T) {
		got := getDomainEdgeAnnotation(newEdge("author", want))
		if got == nil || got.JSONKey != "written_by" || !got.hasScope(ScopeResponse) {
			t.Fatalf("value representation not read: %#v", got)
		}
	})
}

func TestGetDomainEdgeAnnotation_AbsentSources(t *testing.T) {
	cases := map[string]*gen.Edge{
		"nil edge":            nil,
		"nil annotations map": {Name: "posts"},
		"key absent":          {Name: "posts", Annotations: gen.Annotations{"Other": 1}},
		"nil value":           {Name: "posts", Annotations: gen.Annotations{"DomainEdge": nil}},
	}
	for name, e := range cases {
		t.Run(name, func(t *testing.T) {
			if got := getDomainEdgeAnnotation(e); got != nil {
				t.Fatalf("want nil, got %#v", got)
			}
		})
	}
}

// TestEdgeBuildersReturnCopies pins the contract DomainField's builders declare
// and this one must match: a builder never mutates its receiver, so a partially
// configured annotation can be reused as a base for several edges.
func TestEdgeBuildersReturnCopies(t *testing.T) {
	base := Edge()
	derived := base.InResponse()

	if len(base.Scopes) != 0 {
		t.Fatalf("InResponse mutated its receiver: base scopes = %#v", base.Scopes)
	}
	if !derived.hasScope(ScopeResponse) {
		t.Fatal("InResponse did not add the response scope to the copy")
	}

	a := derived.As("a")
	b := derived.As("b")
	if derived.JSONKey != "" {
		t.Fatalf("As mutated its receiver: %q", derived.JSONKey)
	}
	if a.JSONKey != "a" || b.JSONKey != "b" {
		t.Fatalf("As did not isolate its copies: a=%q b=%q", a.JSONKey, b.JSONKey)
	}

	// Two edges built from one base must not share a scope backing array.
	x := derived.InResponse()
	y := derived.InResponse()
	if len(x.Scopes) != 1 || len(y.Scopes) != 1 {
		t.Fatalf("InResponse is not idempotent: x=%#v y=%#v", x.Scopes, y.Scopes)
	}
}

func TestDomainEdgeName(t *testing.T) {
	if got := (DomainEdge{}).Name(); got != "DomainEdge" {
		t.Fatalf("annotation name = %q; the reader looks up %q", got, "DomainEdge")
	}
}
