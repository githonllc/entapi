package fixture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/githonllc/entdomain/internal/fixture/ent"
	"github.com/githonllc/entdomain/internal/fixture/ent/dto"
	"github.com/githonllc/entdomain/internal/fixture/ent/user"
	entdomain "github.com/githonllc/entdomain/runtime"

	stdsql "database/sql"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newClient(t *testing.T) (*ent.Client, context.Context) {
	t.Helper()
	// A per-test DSN. A shared one only works because tests happen to run
	// sequentially and the in-memory database is dropped when the last
	// connection closes — that is luck, not isolation.
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := stdsql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	c := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	t.Cleanup(func() { c.Close() })
	ctx := context.Background()
	if err := c.Schema.Create(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return c, ctx
}

func seed(t *testing.T, c *ent.Client, ctx context.Context) {
	t.Helper()
	rows := []struct {
		name, email, pw string
		status          user.Status
		age             *int
	}{
		{"alice", "alice@a.io", "h1", user.StatusActive, ptr(30)},
		{"alicia", "alicia@b.io", "h2", user.StatusBanned, ptr(41)},
		{"bob", "bob@a.io", "h3", user.StatusActive, nil},
		{"carol", "carol@c.io", "h4", user.StatusActive, ptr(25)},
		{"dave", "dave-alice@d.io", "h5", user.StatusBanned, ptr(50)},
	}
	for _, r := range rows {
		req := &dto.UserCreateRequest{Name: r.name, Email: r.email, PasswordHash: r.pw, Age: r.age}
		s := r.status
		req.Status = &s
		v, err := req.Validate()
		if err != nil {
			t.Fatalf("validate %s: %v", r.name, err)
		}
		if _, err := v.Apply(c.User.Create()).Save(ctx); err != nil {
			t.Fatalf("save %s: %v", r.name, err)
		}
	}
}

func ptr[T any](v T) *T { return &v }

func TestFilterSearchSortPaginate(t *testing.T) {
	c, ctx := newClient(t)
	seed(t, c, ctx)

	t.Run("filter by enum", func(t *testing.T) {
		active := user.StatusActive
		p, err := dto.ListUsers(ctx, c, &dto.UserFilter{Status: &active}, entdomain.ListRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if p.Total != 3 {
			t.Fatalf("want 3 active, got %d", p.Total)
		}
	})

	t.Run("free-text q ORs across searchable fields", func(t *testing.T) {
		// "alice" hits name=alice, name=alicia(no), email=alice@a.io, email=dave-alice@d.io
		p, err := dto.ListUsers(ctx, c, &dto.UserFilter{Q: ptr("alice")}, entdomain.ListRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if p.Total != 2 {
			t.Fatalf("want 2 (alice by name+email, dave by email), got %d", p.Total)
		}
	})

	t.Run("q ANDs with filters", func(t *testing.T) {
		banned := user.StatusBanned
		p, err := dto.ListUsers(ctx, c, &dto.UserFilter{Q: ptr("alice"), Status: &banned}, entdomain.ListRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if p.Total != 1 || p.Data[0].Name != "dave" {
			t.Fatalf("want only banned dave, got total=%d data=%+v", p.Total, p.Data)
		}
	})

	t.Run("sort by allow-listed field", func(t *testing.T) {
		p, err := dto.ListUsers(ctx, c, nil, entdomain.ListRequest{SortBy: "name", Order: "desc"})
		if err != nil {
			t.Fatal(err)
		}
		if p.Data[0].Name != "dave" {
			t.Fatalf("want dave first on name desc, got %s", p.Data[0].Name)
		}
	})

	t.Run("sort by non-allow-listed field is rejected", func(t *testing.T) {
		_, err := dto.ListUsers(ctx, c, nil, entdomain.ListRequest{SortBy: "password_hash"})
		if !errors.Is(err, entdomain.ErrValidation) {
			t.Fatalf("want ErrValidation for password_hash sort, got %v", err)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		p, err := dto.ListUsers(ctx, c, nil, entdomain.ListRequest{SortBy: "name", Size: 2, Page: 2})
		if err != nil {
			t.Fatal(err)
		}
		if p.Total != 5 || len(p.Data) != 2 || p.Data[0].Name != "bob" {
			t.Fatalf("want total=5 len=2 first=bob, got total=%d len=%d first=%s", p.Total, len(p.Data), p.Data[0].Name)
		}
	})

	t.Run("response omits InputOnly field", func(t *testing.T) {
		p, _ := dto.ListUsers(ctx, c, nil, entdomain.ListRequest{})
		b, _ := json.Marshal(p.Data[0])
		if bytesContains(b, "password") {
			t.Fatalf("password leaked into response: %s", b)
		}
	})
}

func bytesContains(b []byte, s string) bool {
	return len(b) > 0 && json.Valid(b) && containsStr(string(b), s)
}
func containsStr(h, n string) bool {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return true
		}
	}
	return false
}

func TestSchemaDefaultSurvivesOmittedField(t *testing.T) {
	c, ctx := newClient(t)
	req := &dto.UserCreateRequest{Name: "eve", Email: "eve@e.io", PasswordHash: "h"}
	v, err := req.Validate()
	if err != nil {
		t.Fatal(err)
	}
	u, err := v.Apply(c.User.Create()).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if u.Status != user.StatusActive {
		t.Fatalf("omitted status must fall back to schema Default(active), got %q", u.Status)
	}
	if u.Age != nil {
		t.Fatalf("omitted optional field must stay null, got %v", *u.Age)
	}
}

func TestPatchThreeStates(t *testing.T) {
	c, ctx := newClient(t)
	base := &dto.UserCreateRequest{Name: "frank", Email: "frank@f.io", PasswordHash: "h", Nickname: ptr("franky")}
	v, _ := base.Validate()
	u, err := v.Apply(c.User.Create()).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}

	apply := func(t *testing.T, body string) *ent.User {
		t.Helper()
		var req dto.UserPatchRequest
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			t.Fatal(err)
		}
		vp, err := req.Validate()
		if err != nil {
			t.Fatal(err)
		}
		got, err := vp.Apply(c.User.UpdateOneID(u.ID)).Save(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	t.Run("absent leaves the field untouched", func(t *testing.T) {
		got := apply(t, `{"name":"frank2"}`)
		if got.Nickname == nil || *got.Nickname != "franky" {
			t.Fatalf("absent nickname must not change, got %v", got.Nickname)
		}
	})

	t.Run("explicit null clears an optional field", func(t *testing.T) {
		got := apply(t, `{"nickname":null}`)
		if got.Nickname != nil {
			t.Fatalf("explicit null must clear, got %q", *got.Nickname)
		}
	})

	t.Run("value sets", func(t *testing.T) {
		got := apply(t, `{"nickname":"f2"}`)
		if got.Nickname == nil || *got.Nickname != "f2" {
			t.Fatalf("value must set, got %v", got.Nickname)
		}
	})

	t.Run("explicit null on a non-optional field is rejected", func(t *testing.T) {
		var req dto.UserPatchRequest
		if err := json.Unmarshal([]byte(`{"name":null}`), &req); err != nil {
			t.Fatal(err)
		}
		if _, err := req.Validate(); !errors.Is(err, entdomain.ErrValidation) {
			t.Fatalf("want ErrValidation for null name, got %v", err)
		}
	})
}
