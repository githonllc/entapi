package fixture

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/githonllc/entapi/internal/fixture/spikeent"
	"github.com/githonllc/entapi/internal/fixture/spikeent/dto"
	entapi "github.com/githonllc/entapi/runtime"
	"github.com/google/uuid"
)

type contextT = context.Context
type uuidT = uuid.UUID

// TestToManyEdgeIsReachable is the case the FK-derived rule could never serve:
// the foreign key lives on Post, so edge.Field() is nil on the User side.
func TestToManyEdgeIsReachable(t *testing.T) {
	c, ctx := newClient(t)
	u := mustUser(t, c, ctx, "author", "author@x.io")
	mustPost(t, c, ctx, u.ID, "first")
	mustPost(t, c, ctx, u.ID, "second")
	lonely := mustUser(t, c, ctx, "lonely", "lonely@x.io")

	got, err := dto.GetUserWithEdges(ctx, c, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Posts) != 2 {
		t.Fatalf("want 2 post summaries, got %d", len(got.Posts))
	}

	t.Run("loaded but empty is an empty array, not an error and not null", func(t *testing.T) {
		got, err := dto.GetUserWithEdges(ctx, c, lonely.ID)
		if err != nil {
			t.Fatalf("loaded-but-empty must not error: %v", err)
		}
		if got.Posts == nil || len(got.Posts) != 0 {
			t.Fatalf("want empty non-nil slice, got %#v", got.Posts)
		}
		b, _ := json.Marshal(got)
		if !strings.Contains(string(b), `"posts":[]`) {
			t.Fatalf(`want "posts":[] in JSON, got %s`, b)
		}
	})
}

// TestNotLoadedFailsLoudly is the whole argument for returning an error: a
// response type declaring an edge must not silently ship without it.
func TestNotLoadedFailsLoudly(t *testing.T) {
	c, ctx := newClient(t)
	u := mustUser(t, c, ctx, "u", "u@x.io")
	mustPost(t, c, ctx, u.ID, "p")

	// db.User.Get is the generated shortcut. It loads no edges.
	raw, err := c.User.Get(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dto.NewUserResponse(raw); err == nil {
		t.Fatal("want an error when the declared edge was never loaded")
	} else if !spikeent.IsNotLoaded(err) {
		t.Fatalf("want NotLoadedError, got %T %v", err, err)
	}
}

// TestToOneEdgeAndScalarAreIndependent: author_id comes from the field scope,
// author from the edge annotation. The old rule made them one switch.
func TestToOneEdgeAndScalarAreIndependent(t *testing.T) {
	c, ctx := newClient(t)
	u := mustUser(t, c, ctx, "writer", "writer@x.io")
	mustPost(t, c, ctx, u.ID, "titled")

	page, err := dto.ListPosts(ctx, c, entapi.ListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 1 {
		t.Fatalf("want 1 post, got %d", len(page.Data))
	}
	p := page.Data[0]
	if p.AuthorID != u.ID {
		t.Fatalf("scalar author_id missing: %v", p.AuthorID)
	}
	if p.Author == nil || p.Author.Name != "writer" {
		t.Fatalf("nested author missing: %#v", p.Author)
	}
}

// TestLoadedButAbsentSerializesAsNull separates the two states JSON tends to
// collapse: "there is no parent" must not look like "nobody asked for it".
func TestLoadedButAbsentSerializesAsNull(t *testing.T) {
	c, ctx := newClient(t)
	root := c.Category.Create().SetName("root").SaveX(ctx)

	got, err := dto.GetCategory(ctx, c, root.ID)
	if err != nil {
		t.Fatalf("loaded-but-absent must not error: %v", err)
	}
	if got.Parent != nil {
		t.Fatalf("want nil parent, got %#v", got.Parent)
	}
	b, _ := json.Marshal(got)
	if !strings.Contains(string(b), `"parent":null`) {
		t.Fatalf(`want an explicit "parent":null, got %s`, b)
	}
}

// TestTwoTierBoundsDepthAndWhatThatCosts documents the limit rather than hiding
// it: a summary carries no edges, so a tree comes back one level deep.
func TestTwoTierBoundsDepthAndWhatThatCosts(t *testing.T) {
	c, ctx := newClient(t)
	root := c.Category.Create().SetName("root").SaveX(ctx)
	mid := c.Category.Create().SetName("mid").SetParentID(root.ID).SaveX(ctx)
	c.Category.Create().SetName("leaf").SetParentID(mid.ID).SaveX(ctx)

	got, err := dto.GetCategory(ctx, c, root.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Children) != 1 || got.Children[0].Name != "mid" {
		t.Fatalf("want one child 'mid', got %#v", got.Children)
	}

	// The cost, stated: "leaf" is unreachable from this response. A summary has
	// no Children field, so there is nowhere for a grandchild to go. Fetching a
	// deeper tree means another round trip per level.
	b, _ := json.Marshal(got)
	if strings.Contains(string(b), "leaf") {
		t.Fatalf("two-tier must not expand past depth 1, but got %s", b)
	}
	if strings.Count(string(b), `"children"`) != 1 {
		t.Fatalf("summary must not carry a children field: %s", b)
	}
}

func mustUser(t *testing.T, c *spikeent.Client, ctx contextT, name, email string) *spikeent.User {
	t.Helper()
	req := &dto.UserCreateRequest{Name: name, Email: email, PasswordHash: "h"}
	v, err := req.Validate()
	if err != nil {
		t.Fatal(err)
	}
	u, err := v.Apply(c.User.Create()).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func mustPost(t *testing.T, c *spikeent.Client, ctx contextT, authorID uuidT, title string) *spikeent.Post {
	t.Helper()
	p, err := c.Post.Create().SetTitle(title).SetAuthorID(authorID).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
