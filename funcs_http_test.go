package entapi

import (
	"reflect"
	"testing"

	"entgo.io/ent/schema/field"

	"github.com/githonllc/entapi/api"
)

func TestResourceOpsFollowExceptAndCreateFamily(t *testing.T) {
	node := newTestType("Article", newStringField("title", nil))
	want := []resourceOperation{
		{Name: api.OpList, Method: "GET", Function: "ListArticles", Field: "listArticles", Handler: "handleListArticles"},
		{Name: api.OpCreate, Method: "POST", Function: "CreateArticle", Field: "createArticle", Handler: "handleCreateArticle"},
		{Name: api.OpGet, Method: "GET", PathSuffix: "/{id}", Function: "GetArticle", Field: "getArticle", Handler: "handleGetArticle"},
		{Name: api.OpPatch, Method: "PATCH", PathSuffix: "/{id}", Function: "PatchArticle", Field: "patchArticle", Handler: "handlePatchArticle"},
		{Name: api.OpDelete, Method: "DELETE", PathSuffix: "/{id}", Function: "DeleteArticle", Field: "deleteArticle", Handler: "handleDeleteArticle"},
	}
	if got := resourceOps(node); !reflect.DeepEqual(got, want) {
		t.Fatalf("resourceOps() = %#v, want %#v", got, want)
	}

	node.Annotations[resourceAnnotationName] = resourcePtr(api.Resource().Except(api.OpDelete, api.OpPatch))
	want = []resourceOperation{want[0], want[1], want[2]}
	if got := resourceOps(node); !reflect.DeepEqual(got, want) {
		t.Fatalf("resourceOps() after Except = %#v, want %#v", got, want)
	}

	blocked := newStringField("secret", fieldPtr(api.Hidden()))
	node = newTestType("Account", blocked)
	node.Annotations[resourceAnnotationName] = resourcePtr(api.Resource().Except(api.OpCreate))
	for _, op := range resourceOps(node) {
		if op.Name == api.OpCreate {
			t.Fatal("resourceOps emits create for a provably unusable Excepted create family")
		}
	}
}

func TestRoutePathUsesEntPluralAndSnake(t *testing.T) {
	if got, want := routePath(newTestType("BlogPost")), "/blog_posts"; got != want {
		t.Errorf("routePath() = %q, want %q", got, want)
	}
}

func TestIDParseExpr(t *testing.T) {
	cases := []struct {
		name string
		id   *field.TypeInfo
		want string
	}{
		{"uuid", &field.TypeInfo{Type: field.TypeUUID, Ident: "uuid.UUID"}, `uuid.Parse(r.PathValue("id"))`},
		{"int", &field.TypeInfo{Type: field.TypeInt, Ident: "int"}, `strconv.ParseInt(r.PathValue("id"), 10, 0)`},
		{"int32", &field.TypeInfo{Type: field.TypeInt32, Ident: "int32"}, `strconv.ParseInt(r.PathValue("id"), 10, 32)`},
		{"string", &field.TypeInfo{Type: field.TypeString, Ident: "string"}, `r.PathValue("id")`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := newField("id", tc.id, nil)
			if got := idParseExpr(id); got != tc.want {
				t.Errorf("idParseExpr() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHandlerImportsAreOperationAndIDSpecific(t *testing.T) {
	node := newTestType("Article", newStringField("title", nil))
	node.ID = newField("id", &field.TypeInfo{
		Type: field.TypeUUID, Ident: "uuid.UUID", PkgPath: "github.com/google/uuid",
	}, nil)
	// The body's own imports left with the bodies in #103: binding, error
	// classification and JSON writing all live behind the entapi alias now, so
	// only a signature or a path-id parse can still name a package here.
	assertImports(t, handlerImports(node),
		`"context"`,
		`"fmt"`,
		`"github.com/google/uuid"`,
		`"net/http"`,
	)

	node.Annotations[resourceAnnotationName] = resourcePtr(api.Resource().Except(api.OpCreate, api.OpPatch, api.OpGet, api.OpDelete))
	assertImports(t, handlerImports(node), `"context"`, `"net/http"`)
}
