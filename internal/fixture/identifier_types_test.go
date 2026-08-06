package fixture

import (
	"testing"

	"github.com/githonllc/entdomain/internal/fixture/spikeent"
	entdomain "github.com/githonllc/entdomain/runtime"
	"github.com/google/uuid"
)

// The identifier type is a type parameter of GetOne, not a hardcoded
// uuid.UUID. One entity proves nothing about that — a signature can be generic
// and still only ever be instantiated one way, which is exactly how
// base_service.tmpl's uuid.UUID assumption survived. So the fixture carries two
// entities with *different* primary key types:
//
//	User.id  uuid.UUID   (field.UUID)
//	Tag.id   int         (ent's default when no id field is declared)
//
// Both call sites below pass no explicit type arguments. If the identifier were
// pinned anywhere in the runtime, one of them would not compile.

type tagView struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func newTagView(t *spikeent.Tag) (*tagView, error) {
	return &tagView{ID: t.ID, Name: t.Name}, nil
}

type userView struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

func newUserView(u *spikeent.User) (*userView, error) {
	return &userView{ID: u.ID, Name: u.Name}, nil
}

func TestGetOneAcceptsAUUIDIdentifier(t *testing.T) {
	c, ctx := newClient(t)
	u := mustUser(t, c, ctx, "uuid-keyed", "uuid@x.io")

	// ID inferred as uuid.UUID from c.User.Get.
	got, err := entdomain.GetOne(ctx, c.User.Get, newUserView, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != u.ID {
		t.Fatalf("id = %v, want %v", got.ID, u.ID)
	}
	var _ uuid.UUID = u.ID
}

func TestGetOneAcceptsAnIntIdentifier(t *testing.T) {
	c, ctx := newClient(t)
	tag := c.Tag.Create().SetName("release").SaveX(ctx)

	// ID inferred as int from c.Tag.Get. Same runtime function, same call
	// shape, a different identifier type.
	got, err := entdomain.GetOne(ctx, c.Tag.Get, newTagView, tag.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != tag.ID || got.Name != "release" {
		t.Fatalf("got %#v, want id=%d name=release", got, tag.ID)
	}
	var _ int = tag.ID
}

// TestListPageDrivesASecondEntBuilder repeats criterion 3 against a builder the
// spike never touched, so "type inference succeeds at the call site" is a
// property of the constraint rather than of one lucky builder.
func TestListPageDrivesASecondEntBuilder(t *testing.T) {
	c, ctx := newClient(t)
	for _, n := range []string{"alpha", "beta", "gamma"} {
		c.Tag.Create().SetName(n).SaveX(ctx)
	}

	page, err := entdomain.ListPage(ctx, c.Tag.Query(), nil, nil,
		entdomain.ListRequest{Size: 2, Page: 2}, newTagView)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 {
		t.Fatalf("Total = %d, want 3", page.Total)
	}
	if len(page.Data) != 1 {
		t.Fatalf("len(Data) = %d, want 1 (page 2 of size 2 over 3 rows)", len(page.Data))
	}
	if page.Size != 2 || page.Page != 2 {
		t.Fatalf("Size/Page = %d/%d, want 2/2", page.Size, page.Page)
	}
}

// TestListPageClampsToMaxPageSizeAgainstEnt checks the bound decided in
// entdomain reaches a real SQL LIMIT, not just the returned metadata.
func TestListPageClampsToMaxPageSizeAgainstEnt(t *testing.T) {
	c, ctx := newClient(t)
	c.Tag.Create().SetName("only").SaveX(ctx)

	page, err := entdomain.ListPage(ctx, c.Tag.Query(), nil, nil,
		entdomain.ListRequest{Size: 10_000}, newTagView)
	if err != nil {
		t.Fatal(err)
	}
	if page.Size != entdomain.MaxPageSize {
		t.Fatalf("Size = %d, want the clamped %d", page.Size, entdomain.MaxPageSize)
	}
}

// TestListPageDoesNotPanicOnANonEmptyTable is #6's carried-forward regression
// asserted where it originally bit: against a real, non-empty table.
func TestListPageDoesNotPanicOnANonEmptyTable(t *testing.T) {
	c, ctx := newClient(t)
	for _, n := range []string{"a", "b", "c"} {
		c.Tag.Create().SetName(n).SaveX(ctx)
	}

	for _, r := range []entdomain.ListRequest{
		{Size: 0},
		{Size: -1},
		{Size: -1, Page: -1},
	} {
		page, err := entdomain.ListPage(ctx, c.Tag.Query(), nil, nil, r, newTagView)
		if err != nil {
			t.Fatalf("%+v: %v", r, err)
		}
		if len(page.Data) != 3 {
			t.Fatalf("%+v: len(Data) = %d, want 3", r, len(page.Data))
		}
	}
}
