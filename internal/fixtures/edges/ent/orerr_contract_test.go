// This file is HAND-WRITTEN and is the one exception to the rule that
// everything under internal/fixtures/<dir>/ent/ is generated. It is not
// regenerated and carries no DO NOT EDIT header.
//
// It has to live in `package ent` because the contract it pins is precisely
// that the loaded/not-loaded flag is UNREACHABLE from anywhere else:
// PostEdges.loadedTypes is unexported, so *loaded and absent* cannot be
// distinguished from *not loaded* by any caller outside this package. Setting
// that flag is the only way to construct the loaded-but-absent state without a
// database, and this module deliberately has no SQL driver — internal/fixture
// (SINGULAR) is the module that exercises these shapes against real SQLite.
//
// `go build` ignores test files, so the codegen harness in
// codegen_fixtures_test.go is unaffected by this file; `go test ./...` from the
// repository root compiles and runs it.
package ent

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestNotLoadedEdgeIsAnError pins the loud half of the contract: a response type
// that declares an edge, converted from an entity that never loaded it, must
// fail rather than ship a response that reads as "this post has no author".
func TestNotLoadedEdgeIsAnError(t *testing.T) {
	t.Run("to-one", func(t *testing.T) {
		p := &Post{ID: uuid.New(), Title: "t", AuthorID: uuid.New()}
		r, err := NewPostResponse(p)
		if err == nil {
			t.Fatalf("expected an error for a not-loaded edge, got response %+v", r)
		}
		if !strings.Contains(err.Error(), "author") {
			t.Errorf("error must name the edge, got %q", err)
		}
	})

	t.Run("to-many", func(t *testing.T) {
		u := &User{ID: uuid.New(), Name: "n"}
		r, err := NewUserResponse(u)
		if err == nil {
			t.Fatalf("expected an error for a not-loaded edge, got response %+v", r)
		}
		if !strings.Contains(err.Error(), "posts") {
			t.Errorf("error must name the edge, got %q", err)
		}
	})
}

// TestLoadedButAbsentIsExplicitNull pins the other half. A to-one edge that was
// loaded and has no related row is a real state, distinct from "nobody asked",
// and it must reach the client as an explicit null rather than a missing key.
func TestLoadedButAbsentIsExplicitNull(t *testing.T) {
	p := &Post{ID: uuid.New(), Title: "t", AuthorID: uuid.New()}
	for i := range p.Edges.loadedTypes {
		p.Edges.loadedTypes[i] = true
	}

	r, err := NewPostResponse(p)
	if err != nil {
		t.Fatalf("loaded-but-absent must not be an error: %v", err)
	}
	if r.Author != nil {
		t.Fatalf("expected a nil author, got %+v", r.Author)
	}

	encoded, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"author":null`) {
		t.Errorf("loaded-but-absent must serialise as an explicit null, got %s", encoded)
	}
}

// TestLoadedToManyEmptyIsAnEmptyArray keeps the third state separate from the
// other two: a to-many edge that was loaded and matched nothing is [], never
// null and never an error.
func TestLoadedToManyEmptyIsAnEmptyArray(t *testing.T) {
	u := &User{ID: uuid.New(), Name: "n"}
	for i := range u.Edges.loadedTypes {
		u.Edges.loadedTypes[i] = true
	}

	r, err := NewUserResponse(u)
	if err != nil {
		t.Fatalf("a loaded-but-empty to-many edge must not error: %v", err)
	}
	if r.Posts == nil {
		t.Fatal("expected an empty slice, got nil")
	}
	if len(r.Posts) != 0 {
		t.Fatalf("expected an empty slice, got %d entries", len(r.Posts))
	}

	encoded, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"posts":[]`) {
		t.Errorf("expected an empty array, got %s", encoded)
	}
}

// TestLoadedEdgePopulatesSummary is the happy path: a loaded edge becomes the
// target's summary, not its full response.
func TestLoadedEdgePopulatesSummary(t *testing.T) {
	author := &User{ID: uuid.New(), Name: "ada"}
	p := &Post{ID: uuid.New(), Title: "t", AuthorID: author.ID}
	p.Edges.Author = author
	for i := range p.Edges.loadedTypes {
		p.Edges.loadedTypes[i] = true
	}

	r, err := NewPostResponse(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Author == nil {
		t.Fatal("expected the author summary to be populated")
	}
	if r.Author.Name != "ada" {
		t.Errorf("Author.Name = %q, want %q", r.Author.Name, "ada")
	}
	if got := reflect.TypeOf(r.Author).String(); got != "*ent.UserSummary" {
		t.Errorf("an edge must carry the summary tier, got %s", got)
	}
}

// TestScalarAndNestedObjectAreIndependent is the whole point of moving exposure
// off the foreign key. reviewer_id is response-scoped and its edge is not
// annotated, so the scalar appears and the nested object does not.
func TestScalarAndNestedObjectAreIndependent(t *testing.T) {
	rt := reflect.TypeOf(PostResponse{})

	for _, name := range []string{"AuthorID", "ReviewerID", "Author"} {
		if _, ok := rt.FieldByName(name); !ok {
			t.Errorf("PostResponse is missing %s", name)
		}
	}
	if _, ok := rt.FieldByName("Reviewer"); ok {
		t.Error("PostResponse carries Reviewer, but the reviewer edge is not annotated")
	}
}

// TestToManyEdgeReachesTheResponse is the case the FK-derived rule could never
// express: the foreign key lives on Post, so edge.Field() is nil on User.
func TestToManyEdgeReachesTheResponse(t *testing.T) {
	f, ok := reflect.TypeOf(UserResponse{}).FieldByName("Posts")
	if !ok {
		t.Fatal("UserResponse has no Posts field")
	}
	if got := f.Type.String(); got != "[]*ent.PostSummary" {
		t.Errorf("Posts is %s, want []*ent.PostSummary", got)
	}
	if got := f.Tag.Get("json"); got != "posts" {
		t.Errorf("json tag is %q; a to-many edge must not be omitempty", got)
	}
}

// TestSummaryTypesCarryNoEdges is the structural bound from QUALITY-REVIEW P1-7.
// It is asserted against the generated types rather than intended: a summary
// with no edge field cannot close a cycle, so depth needs no runtime counter.
func TestSummaryTypesCarryNoEdges(t *testing.T) {
	summaries := []reflect.Type{
		reflect.TypeOf(UserSummary{}),
		reflect.TypeOf(PostSummary{}),
		reflect.TypeOf(CategorySummary{}),
		reflect.TypeOf(SecretSummary{}),
	}
	for _, rt := range summaries {
		for i := 0; i < rt.NumField(); i++ {
			ft := rt.Field(i).Type.String()
			if strings.Contains(ft, "Summary") || strings.Contains(ft, "Response") {
				t.Errorf("%s.%s is %s; summaries must carry no edge fields",
					rt.Name(), rt.Field(i).Name, ft)
			}
		}
	}
}

// TestMutuallyResponseScopedEdgesTerminate is P1-7 itself. User.posts and
// Post.author are both response-scoped and point at each other; with both ends
// loaded the conversion must terminate, which it does because the second level
// is a summary and summaries call nothing.
func TestMutuallyResponseScopedEdgesTerminate(t *testing.T) {
	author := &User{ID: uuid.New(), Name: "ada"}
	post := &Post{ID: uuid.New(), Title: "t", AuthorID: author.ID}

	post.Edges.Author = author
	for i := range post.Edges.loadedTypes {
		post.Edges.loadedTypes[i] = true
	}
	author.Edges.Posts = []*Post{post}
	for i := range author.Edges.loadedTypes {
		author.Edges.loadedTypes[i] = true
	}

	r, err := NewUserResponse(author)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.Posts) != 1 {
		t.Fatalf("expected one post, got %d", len(r.Posts))
	}
	if _, err := NewPostResponse(post); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestTwoTierBoundsDepthAndWhatThatCosts states the cost rather than glossing
// it: a three-level category tree comes back one level deep, and the second
// level has no children field to fill.
func TestTwoTierBoundsDepthAndWhatThatCosts(t *testing.T) {
	grandchild := &Category{ID: uuid.New(), Name: "grandchild"}
	child := &Category{ID: uuid.New(), Name: "child"}
	root := &Category{ID: uuid.New(), Name: "root"}

	child.Edges.Children = []*Category{grandchild}
	root.Edges.Children = []*Category{child}
	for _, c := range []*Category{root, child, grandchild} {
		for i := range c.Edges.loadedTypes {
			c.Edges.loadedTypes[i] = true
		}
	}

	r, err := NewCategoryResponse(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.Children) != 1 {
		t.Fatalf("expected one child, got %d", len(r.Children))
	}
	if _, ok := reflect.TypeOf(*r.Children[0]).FieldByName("Children"); ok {
		t.Error("CategorySummary carries Children; the tree would then be unbounded")
	}
}

// TestEagerLoadPlanCoversExactlyTheResponseEdges is what makes "not loaded is an
// error" cheap: the plan is derived from the response type's edge set, so
// generated wiring cannot forget an edge, and the error only ever catches a
// hand-rolled query.
func TestEagerLoadPlanCoversExactlyTheResponseEdges(t *testing.T) {
	client := NewClient()

	pq := PostQueryWithResponseEdges(client.Post.Query())
	if pq.withAuthor == nil {
		t.Error("the Post plan does not load the annotated author edge")
	}
	if pq.withReviewer != nil {
		t.Error("the Post plan loads reviewer, which no response field needs")
	}

	uq := UserQueryWithResponseEdges(client.User.Query())
	if uq.withPosts == nil {
		t.Error("the User plan does not load the annotated posts edge")
	}
	if uq.withReviewed != nil {
		t.Error("the User plan loads reviewed, which no response field needs")
	}

	cq := CategoryQueryWithResponseEdges(client.Category.Query())
	if cq.withParent == nil || cq.withChildren == nil {
		t.Error("the Category plan must load both ends of the self-referential pair")
	}
}

// TestEntityWithNoResponseFieldsStillHasAResponse is the shape transferred from
// #9: every annotated field on Secret is InputOnly, so the response carries only
// the ID — and ListResponse referring to it must still compile.
func TestEntityWithNoResponseFieldsStillHasAResponse(t *testing.T) {
	id := uuid.New()
	r, err := NewSecretResponse(&Secret{ID: id, Token: "hunter2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.ID != id {
		t.Errorf("ID = %v, want %v", r.ID, id)
	}

	encoded, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(encoded), "hunter2") {
		t.Errorf("an InputOnly field reached a response: %s", encoded)
	}

	var list SecretListResponse
	list.Data = append(list.Data, r)
	if len(list.Data) != 1 {
		t.Fatal("SecretListResponse does not accept its own response type")
	}
}
