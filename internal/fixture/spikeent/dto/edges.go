package dto

import (
	"context"

	"github.com/githonllc/entapi/internal/fixture/spikeent"
	"github.com/githonllc/entapi/internal/fixture/spikeent/category"
	"github.com/githonllc/entapi/internal/fixture/spikeent/user"
	entapi "github.com/githonllc/entapi/runtime"
	"github.com/google/uuid"
)

// ════════════════════════════════════════════════════════════════════════════
// Summary tier — the type a response embeds for its edges. Summaries carry no
// edges of their own, so expansion terminates in the type system rather than at
// a runtime depth counter. A cycle cannot be constructed.
// ════════════════════════════════════════════════════════════════════════════

type UserSummary struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type PostSummary struct {
	ID    uuid.UUID `json:"id"`
	Title string    `json:"title"`
}

type CategorySummary struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

func NewUserSummary(e *spikeent.User) *UserSummary {
	if e == nil {
		return nil
	}
	return &UserSummary{ID: e.ID, Name: e.Name}
}

func NewPostSummary(e *spikeent.Post) *PostSummary {
	if e == nil {
		return nil
	}
	return &PostSummary{ID: e.ID, Title: e.Title}
}

func NewCategorySummary(e *spikeent.Category) *CategorySummary {
	if e == nil {
		return nil
	}
	return &CategorySummary{ID: e.ID, Name: e.Name}
}

// ════════════════════════════════════════════════════════════════════════════
// Post — the to-one edge case.
//
// author_id and author are independent: the scalar comes from the field's
// scope, the nested object from the edge's own annotation. Under the old rule
// both were the same switch.
// ════════════════════════════════════════════════════════════════════════════

type PostResponse struct {
	ID       uuid.UUID `json:"id"`
	Title    string    `json:"title"`
	AuthorID uuid.UUID `json:"author_id"`
	// Not omitempty: a loaded-but-absent edge must serialize as an explicit
	// null. Omitting it would be indistinguishable from "not loaded".
	Author *UserSummary `json:"author"`
}

func NewPostResponse(e *spikeent.Post) (*PostResponse, error) {
	if e == nil {
		return nil, nil
	}
	r := &PostResponse{ID: e.ID, Title: e.Title, AuthorID: e.AuthorID}

	author, err := e.Edges.AuthorOrErr()
	switch {
	case err == nil:
		r.Author = NewUserSummary(author)
	case spikeent.IsNotFound(err):
		// Loaded, and there is no related row. A real state; report it as null.
		r.Author = nil
	default:
		// Not loaded. The caller declared a response type containing this edge
		// and then did not fetch it. Fail loudly instead of shipping a response
		// that looks like "this post has no author".
		return nil, err
	}
	return r, nil
}

// ════════════════════════════════════════════════════════════════════════════
// Category — the self-referential case, where any fixed depth is wrong.
// ════════════════════════════════════════════════════════════════════════════

type CategoryResponse struct {
	ID       uuid.UUID          `json:"id"`
	Name     string             `json:"name"`
	ParentID *uuid.UUID         `json:"parent_id"`
	Parent   *CategorySummary   `json:"parent"`
	Children []*CategorySummary `json:"children"`
}

func NewCategoryResponse(e *spikeent.Category) (*CategoryResponse, error) {
	if e == nil {
		return nil, nil
	}
	r := &CategoryResponse{ID: e.ID, Name: e.Name}
	if e.ParentID != uuid.Nil {
		id := e.ParentID
		r.ParentID = &id
	}

	parent, err := e.Edges.ParentOrErr()
	switch {
	case err == nil:
		r.Parent = NewCategorySummary(parent)
	case spikeent.IsNotFound(err):
		r.Parent = nil
	default:
		return nil, err
	}

	children, err := e.Edges.ChildrenOrErr()
	if err != nil {
		return nil, err
	}
	r.Children = make([]*CategorySummary, 0, len(children))
	for _, c := range children {
		r.Children = append(r.Children, NewCategorySummary(c))
	}
	return r, nil
}

// ════════════════════════════════════════════════════════════════════════════
// Eager-load plans — generated from the response type's edge set, so a caller
// cannot forget one. This is what makes "not loaded is an error" cheap: in
// generated wiring it never fires, and it only ever catches a hand-rolled query.
// ════════════════════════════════════════════════════════════════════════════

func UserQueryWithResponseEdges(q *spikeent.UserQuery) *spikeent.UserQuery {
	return q.WithPosts()
}

func PostQueryWithResponseEdges(q *spikeent.PostQuery) *spikeent.PostQuery {
	return q.WithAuthor()
}

func CategoryQueryWithResponseEdges(q *spikeent.CategoryQuery) *spikeent.CategoryQuery {
	return q.WithParent().WithChildren()
}

// ════════════════════════════════════════════════════════════════════════════
// Wiring.
//
// Note what is NOT used: db.User.Get(ctx, id). The generated Get shortcut loads
// no edges, so it cannot serve a response type that declares any. Wiring for
// such an entity must go through Query with the eager-load plan applied.
// ════════════════════════════════════════════════════════════════════════════

func GetUserWithEdges(ctx context.Context, db *spikeent.Client, id uuid.UUID) (*UserResponse, error) {
	e, err := UserQueryWithResponseEdges(db.User.Query()).Where(user.IDEQ(id)).Only(ctx)
	if err != nil {
		return nil, err
	}
	return NewUserResponse(e)
}

func ListPosts(ctx context.Context, db *spikeent.Client, r entapi.ListRequest) (*entapi.Page[PostResponse], error) {
	return entapi.ListPage(ctx, PostQueryWithResponseEdges(db.Post.Query()), nil, nil, r, NewPostResponse)
}

func GetCategory(ctx context.Context, db *spikeent.Client, id uuid.UUID) (*CategoryResponse, error) {
	e, err := CategoryQueryWithResponseEdges(db.Category.Query()).Where(category.IDEQ(id)).Only(ctx)
	if err != nil {
		return nil, err
	}
	return NewCategoryResponse(e)
}
