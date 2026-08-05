package entdomain

import "fmt"

const (
	// DefaultPageSize is the number of items per page used when a request does
	// not ask for one, or asks for a non-positive one.
	DefaultPageSize = 20

	// MaxPageSize is the maximum allowed number of items per page, and the
	// single place that bound is decided.
	//
	// Everything that enforces a page-size ceiling reads this constant:
	// [ListRequest.Limit] clamps to it and [ListRequest.Validate] rejects above
	// it. The struct tag on ListRequest.Size deliberately does not restate it —
	// a tag cannot reference a constant, so a number written there can only
	// drift out of agreement with this one. It previously said max=100 while
	// this constant said 1000.
	MaxPageSize = 1000
)

// ListRequest represents a paginated list request with optional sorting.
// Supports both offset-based (Page/Size) and cursor-based (Cursor/Size) pagination.
// When Cursor is set, keyset pagination is used; otherwise offset pagination applies.
//
// Size carries no numeric ceiling in its validate tag on purpose; see
// [MaxPageSize].
type ListRequest struct {
	Size   int    `json:"size,omitempty" form:"size" validate:"omitempty,min=1"`
	Page   int    `json:"page,omitempty" form:"page" validate:"omitempty,min=0"`
	SortBy string `json:"sort_by,omitempty" form:"sort_by"`
	Order  string `json:"order,omitempty" form:"order" validate:"omitempty,oneof=asc desc"`
	Cursor string `json:"cursor,omitempty" form:"cursor"` // opaque cursor for keyset pagination
}

// SetDefaults fills in zero-valued fields with sensible defaults.
// Call this before using the request to ensure pagination works correctly.
func (r *ListRequest) SetDefaults() {
	if r.Size == 0 {
		r.Size = DefaultPageSize
	}
}

// Validate checks that all fields are within acceptable bounds.
// It does NOT modify the receiver — call SetDefaults first if needed.
func (r *ListRequest) Validate() error {
	if r == nil {
		return fmt.Errorf("list request cannot be nil")
	}

	if r.Size < 0 {
		return fmt.Errorf("size cannot be negative")
	}
	if r.Size > MaxPageSize {
		return fmt.Errorf("size cannot exceed %d", MaxPageSize)
	}

	if r.Page < 0 {
		return fmt.Errorf("page cannot be negative")
	}

	if r.Order != "" && r.Order != "asc" && r.Order != "desc" {
		return fmt.Errorf("order must be 'asc' or 'desc'")
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
