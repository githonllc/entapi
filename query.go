package entdomain

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// This file is Layer 2: the CRUD algorithms, written once, in Go, with type
// checking, tests and a debugger. Nothing here is generated and nothing here
// imports ent — the entity-specific parts arrive as type parameters and
// function values supplied by generated wiring.

// Query is the subset of an ent query builder that pagination needs.
//
// Q is self-referential because ent's chainable methods return the concrete
// builder type (see entgo.io/ent entc/gen/template/builder/query.tmpl:43-68).
// P is the entity's predicate type, O its order-option type, E the entity.
type Query[Q, P, O, E any] interface {
	Where(...P) Q
	Order(...O) Q
	Limit(int) Q
	Offset(int) Q
	All(context.Context) ([]*E, error)
	Count(context.Context) (int, error)
}

// Page is one page of converted results.
type Page[R any] struct {
	Data  []*R `json:"data"`
	Total int  `json:"total"`
	Page  int  `json:"page"`
	Size  int  `json:"size"`
}

// Limit returns the effective page size: the requested size clamped to
// [MaxPageSize], or [DefaultPageSize] when nothing usable was requested.
//
// It never returns zero or a negative number, which is what keeps a caller from
// reproducing the v1 lister's panic — a zero limit there produced an empty
// slice that was then indexed at len-1.
//
// The bound itself lives in exactly one place, [MaxPageSize]; this function
// reads it rather than restating it.
func (r ListRequest) Limit() int {
	switch {
	case r.Size <= 0:
		return DefaultPageSize
	case r.Size > MaxPageSize:
		return MaxPageSize
	default:
		return r.Size
	}
}

// Offset returns the row offset for offset-based pagination.
func (r ListRequest) Offset() int {
	if r.Page <= 1 {
		return 0
	}
	return (r.Page - 1) * r.Limit()
}

// SortKey validates the requested sort field against an allow-list and reports
// the direction. An empty request falls back to def.
//
// The allow-list is the whole point: an unchecked sort field is an injection
// site, an unindexed-scan trigger, and — combined with paging — an ordering
// oracle over columns the caller was never meant to read.
func (r ListRequest) SortKey(allow []string, def string) (key string, desc bool, err error) {
	key = r.SortBy
	if key == "" {
		key = def
	}
	if key == "" {
		return "", false, nil
	}
	if !slices.Contains(allow, key) {
		return "", false, fmt.Errorf("%w: cannot sort by %q", ErrValidation, key)
	}
	return key, strings.EqualFold(r.Order, "desc"), nil
}

// ListPage runs a filtered, ordered, paginated query and converts the results.
//
// Written once. Every entity reuses it; the generated wiring only supplies the
// builder, the predicates, the order options and the converter.
func ListPage[Q Query[Q, P, O, E], P, O, E, R any](
	ctx context.Context,
	q Q,
	ps []P,
	os []O,
	r ListRequest,
	to func(*E) (*R, error),
) (*Page[R], error) {
	total, err := q.Where(ps...).Count(ctx)
	if err != nil {
		return nil, err
	}

	limit, offset := r.Limit(), r.Offset()
	entities, err := q.Where(ps...).Order(os...).Limit(limit).Offset(offset).All(ctx)
	if err != nil {
		return nil, err
	}

	data := make([]*R, 0, len(entities))
	for _, e := range entities {
		converted, err := to(e)
		if err != nil {
			return nil, err
		}
		data = append(data, converted)
	}

	page := r.Page
	if page <= 0 {
		page = 1
	}
	return &Page[R]{Data: data, Total: total, Page: page, Size: limit}, nil
}

// Saver is the subset of an ent mutation builder a create or an update needs.
//
// Both *<T>Create and *<T>UpdateOne carry it, which is what lets one routine
// serve both operations. It is an interface rather than a plain function value
// because the generated wiring passes the builder itself — v.Apply(...) returns
// the builder, so the call stays a single expression.
type Saver[E any] interface {
	Save(context.Context) (*E, error)
}

// SaveOne runs a mutation builder and converts the entity it returns.
//
// E and R are inferred from to, and the builder is then checked against
// Saver[E]; nothing here names an ent type.
//
// The converter is a parameter rather than a fixed New<T>Response call because
// an entity whose response declares edges cannot be converted from the value
// Save returns — a builder loads no edges. The generated wiring passes a
// converter that re-reads through the eager-load plan in that case, and the
// plain constructor otherwise. Which of the two is a fact about the schema, so
// it is decided at generation time rather than by a branch at runtime.
func SaveOne[B Saver[E], E, R any](
	ctx context.Context,
	b B,
	to func(*E) (*R, error),
) (*R, error) {
	e, err := b.Save(ctx)
	if err != nil {
		return nil, err
	}
	return to(e)
}

// GetOne fetches a single entity by id and converts it.
//
// ID is a type parameter, so the identifier type comes from the schema rather
// than being hardcoded — the generated base service pinned it to uuid.UUID
// only because text/template cannot express generics.
func GetOne[E, R, ID any](
	ctx context.Context,
	get func(context.Context, ID) (*E, error),
	to func(*E) (*R, error),
	id ID,
) (*R, error) {
	e, err := get(ctx, id)
	if err != nil {
		return nil, err
	}
	return to(e)
}
