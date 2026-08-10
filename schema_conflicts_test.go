package entapi

import (
	"strings"
	"testing"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"

	"github.com/githonllc/entapi/api"
)

func conflictText(t *testing.T, nodes ...*gen.Type) string {
	t.Helper()
	err := checkGraphConflicts(&gen.Graph{Nodes: nodes})
	if err == nil {
		t.Fatal("checkGraphConflicts succeeded, want refusal")
	}
	return err.Error()
}

func requireConflict(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("conflict does not contain %q:\n%s", want, got)
		}
	}
}

func TestCheckGraphConflicts_BlockedCreateMatrix(t *testing.T) {
	for _, word := range []struct {
		name       string
		annotation api.FieldAnnotation
	}{
		{"Hidden", api.Hidden()},
		{"ReadOnly", api.ReadOnly()},
	} {
		t.Run(word.name, func(t *testing.T) {
			blocked := newStringField("secret", fieldPtr(word.annotation))
			plain := newStringField("name", nil)
			node := newTestType("Account", blocked, plain)
			got := conflictText(t, node)
			requireConflict(t, got, "Account.secret", "required by Ent", "api."+word.name+"()",
				"Except(api.OpCreate)", "Optional", "Default", "removing api."+word.name+"()")

			node.Annotations[resourceAnnotationName] = resourcePtr(api.Resource().Except(api.OpCreate))
			if err := checkGraphConflicts(&gen.Graph{Nodes: []*gen.Type{node}}); err != nil {
				t.Fatalf("Except(OpCreate) did not repair blocked create: %v", err)
			}
		})
	}
}

func TestCheckGraphConflicts_EmptyPatchMatrix(t *testing.T) {
	immutable := newStringField("slug", nil)
	immutable.Immutable = true
	node := newTestType("Document", immutable)
	got := conflictText(t, node)
	requireConflict(t, got, "Document", "PATCH field set is empty", "Except(api.OpPatch)")

	node.Annotations[resourceAnnotationName] = resourcePtr(api.Resource().Except(api.OpPatch))
	if err := checkGraphConflicts(&gen.Graph{Nodes: []*gen.Type{node}}); err != nil {
		t.Fatalf("Except(OpPatch) did not repair empty patch: %v", err)
	}
}

func TestCheckGraphConflicts_FieldWordMatrix(t *testing.T) {
	plain := newStringField("plain", nil)
	hidden := newStringField("secret", fieldPtr(api.FieldAnnotation{Hidden: true, Sortable: true}))
	readOnlyQuery := newStringField("created_at", fieldPtr(api.FieldAnnotation{ReadOnly: true, Filterable: true, Sortable: true}))
	readOnlyQuery.Optional = true
	node := newTestType("Record", hidden, plain, readOnlyQuery)

	got := conflictText(t, node)
	requireConflict(t, got, "Record.secret", "api.Hidden() conflicts", "Sortable")
	if strings.Contains(got, "Record.created_at: api.Hidden") || strings.Contains(got, "Record.created_at: Ent marks") {
		t.Fatalf("ReadOnly x query dimensions must be allowed:\n%s", got)
	}
}

func TestCheckGraphConflicts_MisplacedWordsAndID(t *testing.T) {
	plain := newStringField("plain", nil)
	plain.Annotations = gen.Annotations{edgeAnnotationName: edgePtr(api.Expand())}
	node := newTestType("Record", plain)
	node.ID.Annotations = gen.Annotations{fieldAnnotationName: fieldPtr(api.Filterable())}
	node.Edges = []*gen.Edge{{
		Name: "owner",
		Type: node,
		// Optional because ent derives that for an edge declared without
		// .Required(); leaving the zero value in would additionally trip the
		// required-edge-without-edge.Field row and stop this test from being
		// about the misplaced word.
		Optional:    true,
		Annotations: gen.Annotations{fieldAnnotationName: fieldPtr(api.Searchable())},
	}}

	got := conflictText(t, node)
	requireConflict(t, got, "Record.id", "primary key", "Record.plain", "Expand() is attached to a field",
		"Record.owner", "field deviation word is attached to an edge")
}

func TestCheckGraphConflicts_QueryRows(t *testing.T) {
	count := newIntField("count", fieldPtr(api.Searchable()))
	plain := newStringField("plain", nil)
	node := newTestType("Record", count, plain)
	node.Annotations[resourceAnnotationName] = resourcePtr(api.Resource().Except(api.OpList))

	got := conflictText(t, node)
	requireConflict(t, got, "Record.count", "Searchable", "no Contains", "Excepts api.OpList")
}

func TestCheckGraphConflicts_FilterableAndSortableCapabilities(t *testing.T) {
	jsonType := &field.TypeInfo{Type: field.TypeJSON, Ident: "map[string]interface{}"}
	jsonField := newField("metadata", jsonType, fieldPtr(api.FieldAnnotation{Filterable: true, Sortable: true}))
	plain := newStringField("plain", nil)
	node := newTestType("Record", jsonField, plain)

	got := conflictText(t, node)
	requireConflict(t, got, "Record.metadata", "Filterable", "no predicates", "Sortable", "not comparable")
}

func TestCheckGraphConflicts_FilterableValueMustBeParseable(t *testing.T) {
	opaque := newField("opaque", &field.TypeInfo{Type: field.TypeBytes, Ident: "[]byte"}, fieldPtr(api.Filterable()))
	node := newTestType("Record", opaque)

	got := conflictText(t, node)
	requireConflict(t, got, "Record.opaque", "Filterable", "wire value", "encoding.TextUnmarshaler")
}

func TestCheckGraphConflicts_PrimaryKeyValueMustBeParseable(t *testing.T) {
	node := newTestType("Record", newStringField("title", nil))
	node.ID = newField("id", &field.TypeInfo{Type: field.TypeBytes, Ident: "[]byte"}, nil)

	got := conflictText(t, node)
	requireConflict(t, got, "Record.id", "primary key", "wire value", "encoding.TextUnmarshaler")
}

func TestCheckGraphConflicts_ExpandTargetMustBeResource(t *testing.T) {
	target := &gen.Type{Name: "User", ID: newIntField("id", nil), Fields: []*gen.Field{newStringField("name", nil)}}
	node := newTestType("Post", newStringField("title", nil))
	node.Edges = []*gen.Edge{{
		Name:        "author",
		Type:        target,
		Optional:    true,
		Annotations: gen.Annotations{edgeAnnotationName: edgePtr(api.Expand())},
	}}
	got := conflictText(t, node, target)
	requireConflict(t, got, "Post.author", "api.Expand() targets User", "not an api.Resource")
}

func TestCheckGraphConflicts_QueryWordsCannotOutliveList(t *testing.T) {
	query := newStringField("name", fieldPtr(api.Filterable()))
	node := newTestType("User", query)
	node.Annotations[resourceAnnotationName] = resourcePtr(api.Resource().Except(api.OpList))
	got := conflictText(t, node)
	requireConflict(t, got, "User.name", "Filterable", "Excepts api.OpList")
}

func TestCheckGraphConflicts_AsymmetricSelfEdgeRemainsRefused(t *testing.T) {
	node := newTestType("Tree", newStringField("name", nil))
	// Both ends Optional, which is what ent derives for a pair declared without
	// .Required(): the zero value would read as Required and pull in the
	// required-edge-without-edge.Field row, which this test is not about.
	assoc := &gen.Edge{Name: "children", Type: node, Optional: true, Annotations: gen.Annotations{edgeAnnotationName: edgePtr(api.Expand())}}
	inverse := &gen.Edge{Name: "parent", Type: node, Optional: true, Inverse: "children", Ref: assoc}
	node.Edges = []*gen.Edge{assoc, inverse}

	got := conflictText(t, node)
	requireConflict(t, got, "Tree.children", "Tree.parent", "self-referential", "api.EdgeAnnotation{}")

	inverse.Annotations = gen.Annotations{edgeAnnotationName: edgePtr(api.Expand())}
	if err := checkGraphConflicts(&gen.Graph{Nodes: []*gen.Type{node}}); err != nil {
		t.Fatalf("symmetric expanded self edge was refused: %v", err)
	}
}

// TestCheckGraphConflicts_RequiredEdgeWithoutFieldMatrix covers both halves of
// #110's message: the remedy list has to offer edge.Field(...) exactly when ent
// would accept it, which is when the edge holds the foreign key.
//
// The third row is what makes the other two mean anything. With only M2O and
// O2M, `unique` and `OwnFK()` co-vary, so an implementation that branched on
// Edge.Unique — the exact mistake requiredEdgeWithoutFieldConflict's comment
// exists to prevent — would pass the whole matrix. The assoc end of a
// two-type O2O pair separates them: Unique is true, OwnFK() is false
// (entc/gen/type.go — O2O owns the key only when IsInverse or Bidi), so a
// Unique-branching implementation fails there and only there.
//
// The hand-built edges carry an explicit gen.Relation because Edge.OwnFK()
// reads Rel.Type, Inverse and Bidi and nothing else. Edge.Field() stays nil in
// every row — the foreign key it would return lives in gen's unexported
// Rel.fk, which is also why the "declares edge.Field()" negative below has to
// load a real graph.
func TestCheckGraphConflicts_RequiredEdgeWithoutFieldMatrix(t *testing.T) {
	target := &gen.Type{Name: "User", ID: newIntField("id", nil)}
	for _, tc := range []struct {
		name string
		// rel is what ent derives for the shape named in the sub-test.
		rel gen.Rel
		// unique mirrors the schema's .Unique(), so the two facts stay
		// independent here the way they are in a real graph.
		unique bool
		// wantFieldRemedy is whether "add edge.Field(...)" is sound advice.
		wantFieldRemedy bool
	}{
		// edge.To("user", User.Type).Unique().Required() with no inverse.
		{name: "to-one holding the foreign key", rel: gen.M2O, unique: true, wantFieldRemedy: true},
		// edge.To("posts", Post.Type).Required(): the key is on the far table.
		{name: "to-many", rel: gen.O2M, wantFieldRemedy: false},
		// The assoc end of edge.To(...).Unique() + edge.From(...).Unique():
		// unique, but the inverse end is the one that may name the key.
		{name: "assoc end of an O2O pair", rel: gen.O2O, unique: true, wantFieldRemedy: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := newTestType("Session", newStringField("token", nil))
			node.Edges = []*gen.Edge{{
				Name:   "user",
				Type:   target,
				Unique: tc.unique,
				Rel:    gen.Relation{Type: tc.rel},
			}}

			got := conflictText(t, node)
			requireConflict(t, got, "Session.user", "Required()", "SessionCreateRequest",
				"Except(api.OpCreate)", "Optional")

			hasFieldRemedy := strings.Contains(got, "by adding edge.Field(")
			if hasFieldRemedy != tc.wantFieldRemedy {
				t.Errorf("edge.Field(...) offered as a remedy = %v, want %v:\n%s", hasFieldRemedy, tc.wantFieldRemedy, got)
			}
		})
	}
}

func TestCheckGraphConflicts_RequiredEdgeWithoutFieldNegatives(t *testing.T) {
	target := &gen.Type{Name: "User", ID: newIntField("id", nil)}

	newSession := func() *gen.Type {
		node := newTestType("Session", newStringField("token", nil))
		node.Edges = []*gen.Edge{{
			Name:   "user",
			Type:   target,
			Unique: true,
			Rel:    gen.Relation{Type: gen.M2O},
		}}
		return node
	}

	t.Run("optional edge", func(t *testing.T) {
		node := newSession()
		node.Edges[0].Optional = true
		if err := checkGraphConflicts(&gen.Graph{Nodes: []*gen.Type{node}}); err != nil {
			t.Fatalf("an Optional edge without edge.Field() was refused: %v", err)
		}
	})

	t.Run("create excepted", func(t *testing.T) {
		node := newSession()
		node.Annotations[resourceAnnotationName] = resourcePtr(api.Resource().Except(api.OpCreate))
		if err := checkGraphConflicts(&gen.Graph{Nodes: []*gen.Type{node}}); err != nil {
			t.Fatalf("Except(OpCreate) did not repair the required edge: %v", err)
		}
	})
}

// TestCheckGraphConflicts_RequiredEdgeWithFieldIsAccepted is the negative that
// cannot be hand-built: gen.Edge.Field() reads Rel.fk, which is unexported and
// is only ever set by gen's own graph resolution. So it loads the "edges"
// fixture — whose Post.author is Required, Unique and declares
// Field("author_id") — through the real loader.
//
// The two positive controls are the point of the test as much as the nil error
// is: an absence assertion over a graph that no longer contains the shape it
// claims to cover would pass for the wrong reason.
func TestCheckGraphConflicts_RequiredEdgeWithFieldIsAccepted(t *testing.T) {
	root := repoRoot(t)
	g, err := entc.LoadGraph(fixtureSchemaDir(root, "edges"), &gen.Config{
		Package: fixtureEntPkgPath("edges"),
	})
	if err != nil {
		t.Fatalf("loading the edges fixture graph: %v", err)
	}

	var author *gen.Edge
	for _, node := range g.Nodes {
		if node.Name != "Post" {
			continue
		}
		for _, e := range node.Edges {
			if e.Name == "author" {
				author = e
			}
		}
	}
	if author == nil {
		t.Fatal("control: the edges fixture no longer has a Post.author edge")
	}
	if author.Optional {
		t.Fatal("control: Post.author is no longer Required(), so this test no longer covers a required edge")
	}
	if author.Field() == nil {
		t.Fatal("control: Post.author no longer declares edge.Field(), so this test no longer covers the accepted shape")
	}

	if err := checkGraphConflicts(g); err != nil {
		t.Fatalf("a Required edge that declares edge.Field() was refused: %v", err)
	}
}

func TestCheckGraphConflicts_NonResourceIsSilent(t *testing.T) {
	node := &gen.Type{Name: "Plain", ID: newIntField("id", nil), Fields: []*gen.Field{newStringField("name", fieldPtr(api.Hidden()))}}
	if err := checkGraphConflicts(&gen.Graph{Nodes: []*gen.Type{node}}); err != nil {
		t.Fatalf("non-Resource node was checked against HTTP matrix: %v", err)
	}
}

func TestCheckGraphConflicts_ReservedNamesUseResources(t *testing.T) {
	resource := newTestType("Widget", newStringField("name", nil))
	for _, name := range []string{"ErrorMap", "API", "APIHandler", "APIOption"} {
		t.Run(name, func(t *testing.T) {
			collision := &gen.Type{Name: name, ID: newIntField("id", nil)}
			got := conflictText(t, resource, collision)
			requireConflict(t, got, name, "rename")
		})
	}
}

func TestCheckGraphConflicts_HandlerFnNamesUseMaximumBreadth(t *testing.T) {
	resource := newTestType("Widget", newStringField("name", nil))
	resource.Annotations[resourceAnnotationName] = resourcePtr(api.Resource().Except(api.OpDelete))
	collision := &gen.Type{Name: "DeleteWidgetFn", ID: newIntField("id", nil)}

	got := conflictText(t, resource, collision)
	requireConflict(t, got, "DeleteWidgetFn", "widget_handler.go", "reserved even if")
}
