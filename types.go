package entdomain

import (
	"fmt"
	"strings"
)

const (
	// DefaultPageSize is the number of items per page used when a request does
	// not ask for one, or asks for a non-positive one.
	DefaultPageSize = 20

	// MaxPageSize is the maximum allowed number of items per page: the single
	// place that bound is decided, with a single reaction to crossing it.
	//
	// [ListRequest.Limit] is the only thing that reads it, and it clamps.
	// Clamping rather than rejecting, because Limit sits on the only path into
	// [ListPage] and therefore applies whether or not a caller remembers to
	// validate anything; a ceiling that fires only when someone opts in is
	// advice, not a bound. Whether an oversized request also deserves a 4xx is
	// a policy the consumer owns — compare against this constant to decide.
	// [Page].Size reports the size actually used, so clamping is visible.
	//
	// The struct tag on ListRequest.Size deliberately does not restate the
	// number — a tag cannot reference a constant, so a number written there can
	// only drift out of agreement with this one. It previously said max=100
	// while this constant said 1000.
	MaxPageSize = 1000
)

// ListRequest represents a paginated list request with optional sorting.
// Supports both offset-based (Page/Size) and cursor-based (Cursor/Size) pagination.
// When Cursor is set, keyset pagination is used; otherwise offset pagination applies.
//
// The zero value is usable and needs no preparation. Read the effective values
// through [ListRequest.Limit] and [ListRequest.Offset], never off the fields
// directly: those two methods are what default and clamp, and they are what
// [ListPage] calls. There is deliberately no defaulting method to forget — a
// zero Size cannot reach a query.
//
// The validate struct tags deliberately carry no rules. A tag cannot reference
// a constant and cannot express a case-insensitive comparison, so every rule
// spelled there is a second spelling that can only drift away from the code
// enforcing it — as max=100 had already drifted from [MaxPageSize]=1000, and
// oneof=asc desc from [ListRequest.SortKey]'s EqualFold. [ListRequest.Validate],
// [ListRequest.Limit] and [ListRequest.Offset] are the homes.
type ListRequest struct {
	Size   int    `json:"size,omitempty" form:"size"`
	Page   int    `json:"page,omitempty" form:"page"`
	SortBy string `json:"sort_by,omitempty" form:"sort_by"`
	Order  string `json:"order,omitempty" form:"order"`
	Cursor string `json:"cursor,omitempty" form:"cursor"` // opaque cursor for keyset pagination
}

// Validate reports input that nothing downstream can silently repair. Errors
// wrap [ErrValidation].
//
// It deliberately says nothing about Size or Page. [ListRequest.Limit] and
// [ListRequest.Offset] normalise those on the only path into [ListPage], so a
// second opinion here would be a bypassable duplicate of a bound that is
// already enforced — and the two used to disagree, one clamping and one
// rejecting. A consumer that wants an oversized request to be a client error
// compares Size against [MaxPageSize] itself.
//
// Order is different: nothing repairs it. [ListRequest.SortKey] reads an
// unrecognised value as ascending rather than failing, so an unnoticed typo
// silently reverses the results.
//
// The comparison is case-insensitive because SortKey's is, and SortKey is what
// decides the direction — it sits on the only path into [ListPage] and applies
// whether or not anyone validates. Validate rejects exactly what SortKey will
// not honour, and no more: "DESC" sorts descending, so refusing it here would
// be this type disagreeing with itself.
func (r *ListRequest) Validate() error {
	if r == nil {
		return fmt.Errorf("%w: list request cannot be nil", ErrValidation)
	}

	if r.Order != "" && !strings.EqualFold(r.Order, "asc") && !strings.EqualFold(r.Order, "desc") {
		return fmt.Errorf("%w: order must be 'asc' or 'desc', got %q", ErrValidation, r.Order)
	}

	return nil
}

// Ptr returns a pointer to the given value.
func Ptr[T any](v T) *T { return &v }

// PtrOrNil returns a pointer to v, or nil if v is the zero value for its type.
func PtrOrNil[T comparable](v T) *T {
	var zero T
	if v == zero {
		return nil
	}
	return &v
}

// PtrNilSafe returns a pointer to v, or nil if v is nil.
// Use for types that are not comparable (maps, slices) where PtrOrNil cannot be used.
func PtrNilSafe[T any](v T) *T {
	if any(v) == nil {
		return nil
	}
	return &v
}
