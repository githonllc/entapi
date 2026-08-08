package entapi

import (
	"strings"
	"testing"

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
		Name:        "owner",
		Type:        node,
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
	assoc := &gen.Edge{Name: "children", Type: node, Annotations: gen.Annotations{edgeAnnotationName: edgePtr(api.Expand())}}
	inverse := &gen.Edge{Name: "parent", Type: node, Inverse: "children", Ref: assoc}
	node.Edges = []*gen.Edge{assoc, inverse}

	got := conflictText(t, node)
	requireConflict(t, got, "Tree.children", "Tree.parent", "self-referential", "issue #79")

	inverse.Annotations = gen.Annotations{edgeAnnotationName: edgePtr(api.Expand())}
	if err := checkGraphConflicts(&gen.Graph{Nodes: []*gen.Type{node}}); err != nil {
		t.Fatalf("symmetric expanded self edge was refused: %v", err)
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
