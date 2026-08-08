// Package dto holds the artifacts the generator is meant to emit.
//
// Everything in this file is HAND-WRITTEN on purpose. The point of the spike is
// to write the target output first, confirm it compiles, reads well and runs,
// and only then teach a template to produce it. Doing that in the other order
// is how the current templates ended up emitting code no test ever compiled.
//
// Every declaration below is mechanical: its shape follows from the schema and
// from ent's own generated API. Nothing here required a judgement call, which
// is the test for whether it belongs in generated code at all.
package dto

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/githonllc/entapi/internal/fixture/spikeent"
	"github.com/githonllc/entapi/internal/fixture/spikeent/predicate"
	"github.com/githonllc/entapi/internal/fixture/spikeent/user"
	entapi "github.com/githonllc/entapi/runtime"
	"github.com/google/uuid"
)

// ════════════════════════════════════════════════════════════════════════════
// Response — fields carrying the response scope. password_hash is InputOnly and
// therefore absent; there is no way to opt it back in from the HTTP layer.
// ════════════════════════════════════════════════════════════════════════════

type UserResponse struct {
	ID        uuid.UUID   `json:"id"`
	Name      string      `json:"name"`
	Email     string      `json:"email"`
	Status    user.Status `json:"status"`
	Age       *int        `json:"age,omitempty"`
	Nickname  *string     `json:"nickname,omitempty"`
	CreatedAt time.Time   `json:"created_at"`

	// To-many edge. Under the FK-derived rule this could never appear here:
	// the foreign key lives on Post, so edge.Field() is nil on this side.
	Posts []*PostSummary `json:"posts"`
}

// NewUserResponse converts an entity to its response DTO.
//
// It returns an error only because entities with response edges must report a
// not-loaded edge rather than silently omitting it. This entity has no edges,
// so the error is always nil here — the signature is kept uniform so the
// generic wiring in Layer 2 does not need two shapes.
func NewUserResponse(e *spikeent.User) (*UserResponse, error) {
	if e == nil {
		return nil, nil
	}
	r := &UserResponse{
		ID:        e.ID,
		Name:      e.Name,
		Email:     e.Email,
		Status:    e.Status,
		Age:       e.Age,
		Nickname:  e.Nickname,
		CreatedAt: e.CreatedAt,
	}
	posts, err := e.Edges.PostsOrErr()
	if err != nil {
		// To-many edges have no not-found state: loaded-but-empty is an empty
		// slice, so any error here means the caller did not load the edge.
		return nil, err
	}
	r.Posts = make([]*PostSummary, 0, len(posts))
	for _, p := range posts {
		r.Posts = append(r.Posts, NewPostSummary(p))
	}
	return r, nil
}

// ════════════════════════════════════════════════════════════════════════════
// Create — parse, don't validate. Apply is defined only on the validated type,
// so reaching Apply without validating is a compile error rather than a
// discipline problem.
// ════════════════════════════════════════════════════════════════════════════

type UserCreateRequest struct {
	Name         string       `json:"name"`
	Email        string       `json:"email"`
	PasswordHash string       `json:"password_hash"`
	Status       *user.Status `json:"status,omitempty"`
	Age          *int         `json:"age,omitempty"`
	Nickname     *string      `json:"nickname,omitempty"`
}

type ValidUserCreateRequest struct{ r *UserCreateRequest }

func (r *UserCreateRequest) Validate() (*ValidUserCreateRequest, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: create request is nil", entapi.ErrValidation)
	}
	if r.Name == "" {
		return nil, fmt.Errorf("%w: name is required", entapi.ErrValidation)
	}
	if r.Email == "" {
		return nil, fmt.Errorf("%w: email is required", entapi.ErrValidation)
	}
	if r.PasswordHash == "" {
		return nil, fmt.Errorf("%w: password_hash is required", entapi.ErrValidation)
	}
	if r.Status != nil {
		if err := user.StatusValidator(*r.Status); err != nil {
			return nil, fmt.Errorf("%w: %v", entapi.ErrValidation, err)
		}
	}
	return &ValidUserCreateRequest{r: r}, nil
}

// Apply writes the request onto a create builder.
//
// Fields the caller omitted are not written at all, so the schema's Default()
// applies. Writing a zero value unconditionally — which the current
// base_service template does — silently defeats every schema default.
func (v *ValidUserCreateRequest) Apply(b *spikeent.UserCreate) *spikeent.UserCreate {
	r := v.r
	b.SetName(r.Name)
	b.SetEmail(r.Email)
	b.SetPasswordHash(r.PasswordHash)
	b.SetNillableStatus(r.Status) // nil => schema Default("active") applies
	b.SetNillableAge(r.Age)       // optional + nillable
	b.SetNillableNickname(r.Nickname)
	// created_at is Immutable with a Default: never settable from the HTTP layer.
	return b
}

// ════════════════════════════════════════════════════════════════════════════
// Patch — three-state semantics via a generated UnmarshalJSON.
//
// Fields stay *T so the wire format, form binders and every reflection-based
// consumer keep working. Presence is recorded separately. A generic
// Optional[T] wrapper was considered and rejected: unexported fields plus no
// MarshalJSON means re-serialising a request emits {} and loses all three
// states.
// ════════════════════════════════════════════════════════════════════════════

type UserPatchRequest struct {
	Name     *string      `json:"name,omitempty"`
	Email    *string      `json:"email,omitempty"`
	Status   *user.Status `json:"status,omitempty"`
	Age      *int         `json:"age,omitempty"`
	Nickname *string      `json:"nickname,omitempty"`

	present map[string]bool
}

func (r *UserPatchRequest) UnmarshalJSON(b []byte) error {
	type alias UserPatchRequest // breaks the UnmarshalJSON recursion
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if err := json.Unmarshal(b, (*alias)(r)); err != nil {
		return err
	}
	r.present = make(map[string]bool, len(raw))
	for k := range raw {
		r.present[k] = true
	}
	return nil
}

func (r *UserPatchRequest) has(field string) bool { return r.present[field] }

func (r *UserPatchRequest) HasName() bool     { return r.has("name") }
func (r *UserPatchRequest) HasEmail() bool    { return r.has("email") }
func (r *UserPatchRequest) HasStatus() bool   { return r.has("status") }
func (r *UserPatchRequest) HasAge() bool      { return r.has("age") }
func (r *UserPatchRequest) HasNickname() bool { return r.has("nickname") }

type ValidUserPatchRequest struct{ r *UserPatchRequest }

// Validate rejects an explicit null on a field the schema declares non-optional.
// Only optional fields can be cleared, because ent emits Clear<Field>() only for
// fields that are Optional and present in MutableFields.
func (r *UserPatchRequest) Validate() (*ValidUserPatchRequest, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: patch request is nil", entapi.ErrValidation)
	}
	for _, f := range []struct {
		name    string
		present bool
		isNull  bool
	}{
		{"name", r.HasName(), r.Name == nil},
		{"email", r.HasEmail(), r.Email == nil},
		{"status", r.HasStatus(), r.Status == nil},
	} {
		if f.present && f.isNull {
			return nil, fmt.Errorf("%w: %s cannot be null", entapi.ErrValidation, f.name)
		}
	}
	if r.HasStatus() && r.Status != nil {
		if err := user.StatusValidator(*r.Status); err != nil {
			return nil, fmt.Errorf("%w: %v", entapi.ErrValidation, err)
		}
	}
	return &ValidUserPatchRequest{r: r}, nil
}

func (v *ValidUserPatchRequest) Apply(b *spikeent.UserUpdateOne) *spikeent.UserUpdateOne {
	r := v.r
	if r.HasName() {
		b.SetName(*r.Name)
	}
	if r.HasEmail() {
		b.SetEmail(*r.Email)
	}
	if r.HasStatus() {
		b.SetStatus(*r.Status)
	}
	// Optional fields: present+null clears, present+value sets, absent is untouched.
	if r.HasAge() {
		if r.Age == nil {
			b.ClearAge()
		} else {
			b.SetAge(*r.Age)
		}
	}
	if r.HasNickname() {
		if r.Nickname == nil {
			b.ClearNickname()
		} else {
			b.SetNickname(*r.Nickname)
		}
	}
	return b
}

// ════════════════════════════════════════════════════════════════════════════
// Filter — one field group per Filterable field. Which operators exist follows
// from the field's type: ent already computes this (entc/gen/func.go fieldOps,
// entc/gen/predicate.go:67-71). enum gets EQ/In, string adds Contains/HasPrefix,
// an optional field gains IsNil.
//
// Q is the free-text search across every Searchable field, OR'd together and
// then AND'd with the rest.
// ════════════════════════════════════════════════════════════════════════════

type UserFilter struct {
	// name: string (not optional) -> stringOps, in full.
	Name             *string  `form:"name" json:"name,omitempty"`
	NameNEQ          *string  `form:"name_neq" json:"name_neq,omitempty"`
	NameIn           []string `form:"name_in" json:"name_in,omitempty"`
	NameNotIn        []string `form:"name_not_in" json:"name_not_in,omitempty"`
	NameGT           *string  `form:"name_gt" json:"name_gt,omitempty"`
	NameGTE          *string  `form:"name_gte" json:"name_gte,omitempty"`
	NameLT           *string  `form:"name_lt" json:"name_lt,omitempty"`
	NameLTE          *string  `form:"name_lte" json:"name_lte,omitempty"`
	NameContains     *string  `form:"name_contains" json:"name_contains,omitempty"`
	NameHasPrefix    *string  `form:"name_prefix" json:"name_prefix,omitempty"`
	NameHasSuffix    *string  `form:"name_suffix" json:"name_suffix,omitempty"`
	NameEqualFold    *string  `form:"name_ieq" json:"name_ieq,omitempty"`
	NameContainsFold *string  `form:"name_icontains" json:"name_icontains,omitempty"`

	// email: string -> stringOps, in full.
	Email             *string  `form:"email" json:"email,omitempty"`
	EmailNEQ          *string  `form:"email_neq" json:"email_neq,omitempty"`
	EmailIn           []string `form:"email_in" json:"email_in,omitempty"`
	EmailNotIn        []string `form:"email_not_in" json:"email_not_in,omitempty"`
	EmailGT           *string  `form:"email_gt" json:"email_gt,omitempty"`
	EmailGTE          *string  `form:"email_gte" json:"email_gte,omitempty"`
	EmailLT           *string  `form:"email_lt" json:"email_lt,omitempty"`
	EmailLTE          *string  `form:"email_lte" json:"email_lte,omitempty"`
	EmailContains     *string  `form:"email_contains" json:"email_contains,omitempty"`
	EmailHasPrefix    *string  `form:"email_prefix" json:"email_prefix,omitempty"`
	EmailHasSuffix    *string  `form:"email_suffix" json:"email_suffix,omitempty"`
	EmailEqualFold    *string  `form:"email_ieq" json:"email_ieq,omitempty"`
	EmailContainsFold *string  `form:"email_icontains" json:"email_icontains,omitempty"`

	// status: enum -> enumOps (EQ, NEQ, In, NotIn). No Contains: ent does not
	// emit substring predicates for enums, so none can be generated here.
	Status      *user.Status  `form:"status" json:"status,omitempty"`
	StatusNEQ   *user.Status  `form:"status_neq" json:"status_neq,omitempty"`
	StatusIn    []user.Status `form:"status_in" json:"status_in,omitempty"`
	StatusNotIn []user.Status `form:"status_not_in" json:"status_not_in,omitempty"`

	// age: int, Optional -> numericOps + nillableOps.
	Age      *int  `form:"age" json:"age,omitempty"`
	AgeNEQ   *int  `form:"age_neq" json:"age_neq,omitempty"`
	AgeIn    []int `form:"age_in" json:"age_in,omitempty"`
	AgeNotIn []int `form:"age_not_in" json:"age_not_in,omitempty"`
	AgeGT    *int  `form:"age_gt" json:"age_gt,omitempty"`
	AgeGTE   *int  `form:"age_gte" json:"age_gte,omitempty"`
	AgeLT    *int  `form:"age_lt" json:"age_lt,omitempty"`
	AgeLTE   *int  `form:"age_lte" json:"age_lte,omitempty"`
	// IsNil and NotNil are one boolean question, so the pair collapses to one
	// parameter. Emitting both separately would admit a contradictory request.
	AgeIsNull *bool `form:"age_is_null" json:"age_is_null,omitempty"`

	// created_at: time.Time -> numericOps.
	CreatedAt      *time.Time  `form:"created_at" json:"created_at,omitempty"`
	CreatedAtNEQ   *time.Time  `form:"created_at_neq" json:"created_at_neq,omitempty"`
	CreatedAtIn    []time.Time `form:"created_at_in" json:"created_at_in,omitempty"`
	CreatedAtNotIn []time.Time `form:"created_at_not_in" json:"created_at_not_in,omitempty"`
	CreatedAtGT    *time.Time  `form:"created_after_excl" json:"created_after_excl,omitempty"`
	CreatedAtGTE   *time.Time  `form:"created_after" json:"created_after,omitempty"`
	CreatedAtLT    *time.Time  `form:"created_before_excl" json:"created_before_excl,omitempty"`
	CreatedAtLTE   *time.Time  `form:"created_before" json:"created_before,omitempty"`

	// Free-text search over the Searchable fields: name, email.
	Q *string `form:"q" json:"q,omitempty"`
}

func (f *UserFilter) Predicates() []predicate.User {
	if f == nil {
		return nil
	}
	var ps []predicate.User

	appendIf(&ps, f.Name, user.NameEQ)
	appendIf(&ps, f.NameNEQ, user.NameNEQ)
	appendIfSlice(&ps, f.NameIn, user.NameIn)
	appendIfSlice(&ps, f.NameNotIn, user.NameNotIn)
	appendIf(&ps, f.NameGT, user.NameGT)
	appendIf(&ps, f.NameGTE, user.NameGTE)
	appendIf(&ps, f.NameLT, user.NameLT)
	appendIf(&ps, f.NameLTE, user.NameLTE)
	appendIf(&ps, f.NameContains, user.NameContains)
	appendIf(&ps, f.NameHasPrefix, user.NameHasPrefix)
	appendIf(&ps, f.NameHasSuffix, user.NameHasSuffix)
	appendIf(&ps, f.NameEqualFold, user.NameEqualFold)
	appendIf(&ps, f.NameContainsFold, user.NameContainsFold)

	appendIf(&ps, f.Email, user.EmailEQ)
	appendIf(&ps, f.EmailNEQ, user.EmailNEQ)
	appendIfSlice(&ps, f.EmailIn, user.EmailIn)
	appendIfSlice(&ps, f.EmailNotIn, user.EmailNotIn)
	appendIf(&ps, f.EmailGT, user.EmailGT)
	appendIf(&ps, f.EmailGTE, user.EmailGTE)
	appendIf(&ps, f.EmailLT, user.EmailLT)
	appendIf(&ps, f.EmailLTE, user.EmailLTE)
	appendIf(&ps, f.EmailContains, user.EmailContains)
	appendIf(&ps, f.EmailHasPrefix, user.EmailHasPrefix)
	appendIf(&ps, f.EmailHasSuffix, user.EmailHasSuffix)
	appendIf(&ps, f.EmailEqualFold, user.EmailEqualFold)
	appendIf(&ps, f.EmailContainsFold, user.EmailContainsFold)

	appendIf(&ps, f.Status, user.StatusEQ)
	appendIf(&ps, f.StatusNEQ, user.StatusNEQ)
	appendIfSlice(&ps, f.StatusIn, user.StatusIn)
	appendIfSlice(&ps, f.StatusNotIn, user.StatusNotIn)

	appendIf(&ps, f.Age, user.AgeEQ)
	appendIf(&ps, f.AgeNEQ, user.AgeNEQ)
	appendIfSlice(&ps, f.AgeIn, user.AgeIn)
	appendIfSlice(&ps, f.AgeNotIn, user.AgeNotIn)
	appendIf(&ps, f.AgeGT, user.AgeGT)
	appendIf(&ps, f.AgeGTE, user.AgeGTE)
	appendIf(&ps, f.AgeLT, user.AgeLT)
	appendIf(&ps, f.AgeLTE, user.AgeLTE)
	if f.AgeIsNull != nil {
		if *f.AgeIsNull {
			ps = append(ps, user.AgeIsNil())
		} else {
			ps = append(ps, user.AgeNotNil())
		}
	}

	appendIf(&ps, f.CreatedAt, user.CreatedAtEQ)
	appendIf(&ps, f.CreatedAtNEQ, user.CreatedAtNEQ)
	appendIfSlice(&ps, f.CreatedAtIn, user.CreatedAtIn)
	appendIfSlice(&ps, f.CreatedAtNotIn, user.CreatedAtNotIn)
	appendIf(&ps, f.CreatedAtGT, user.CreatedAtGT)
	appendIf(&ps, f.CreatedAtGTE, user.CreatedAtGTE)
	appendIf(&ps, f.CreatedAtLT, user.CreatedAtLT)
	appendIf(&ps, f.CreatedAtLTE, user.CreatedAtLTE)

	// Searchable: one input, OR across the searchable fields, AND'd with the rest.
	if f.Q != nil && *f.Q != "" {
		ps = append(ps, user.Or(
			user.NameContains(*f.Q),
			user.EmailContains(*f.Q),
		))
	}

	return ps
}

// appendIf and appendIfSlice keep the full operator set from turning into a
// wall of identical four-line if statements. They live here rather than in the
// runtime because they are typed to this entity's predicate type.
func appendIf[T any](ps *[]predicate.User, v *T, f func(T) predicate.User) {
	if v != nil {
		*ps = append(*ps, f(*v))
	}
}

func appendIfSlice[T any](ps *[]predicate.User, vs []T, f func(...T) predicate.User) {
	if len(vs) > 0 {
		*ps = append(*ps, f(vs...))
	}
}

// ════════════════════════════════════════════════════════════════════════════
// Sortable — an allow-list. Fields not annotated cannot be ordered by, which is
// where injection, unindexed scans and paging-as-an-ordering-oracle are stopped.
// password_hash is deliberately absent.
// ════════════════════════════════════════════════════════════════════════════

var userSortable = map[string]func(...sql.OrderTermOption) user.OrderOption{
	"name":       user.ByName,
	"created_at": user.ByCreatedAt,
}

// UserSortKeys is the allow-list handed to the runtime validator.
var UserSortKeys = []string{"name", "created_at"}

func UserOrder(r entapi.ListRequest) ([]user.OrderOption, error) {
	key, desc, err := r.SortKey(UserSortKeys, "created_at")
	if err != nil {
		return nil, err
	}
	by, ok := userSortable[key]
	if !ok {
		return nil, nil
	}
	if desc {
		return []user.OrderOption{by(sql.OrderDesc())}, nil
	}
	return []user.OrderOption{by(sql.OrderAsc())}, nil
}

// ════════════════════════════════════════════════════════════════════════════
// Wiring — one line per operation. The algorithm lives in Layer 2; these only
// connect this entity's builders, predicates, order options and converter to it.
//
// Free functions, not methods on a generated struct: there is nothing to embed,
// nothing to override, and no SetSelf to forget. A consumer who needs different
// behaviour writes their own function and stops calling this one.
// ════════════════════════════════════════════════════════════════════════════

func ListUsers(ctx context.Context, db *spikeent.Client, f *UserFilter, r entapi.ListRequest) (*entapi.Page[UserResponse], error) {
	os, err := UserOrder(r)
	if err != nil {
		return nil, err
	}
	q := UserQueryWithResponseEdges(db.User.Query())
	return entapi.ListPage(ctx, q, f.Predicates(), os, r, NewUserResponse)
}

func GetUser(ctx context.Context, db *spikeent.Client, id uuid.UUID) (*UserResponse, error) {
	return entapi.GetOne(ctx, db.User.Get, NewUserResponse, id)
}
