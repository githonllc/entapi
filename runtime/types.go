package entapi

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
//
// Pagination is offset-based, and only offset-based: Page and Size are the
// whole of it. There used to be a Cursor field here, documented as "when Cursor
// is set, keyset pagination is used", and no code anywhere branched on it — a
// consumer who believed the comment got offset page one, silently, forever. It
// was removed on #6 along with the base64(json) codec that named its format;
// see the README migration note. Adding keyset paging later is additive, which
// is the asymmetry that decided it.
//
// The zero value is usable and needs no preparation. Read the effective values
// through [ListRequest.Limit] and [ListRequest.Offset], never off the fields
// directly: those two methods are what default and clamp, and they are what
// [ListPage] calls. There is deliberately no defaulting method to forget — a
// zero Size cannot reach a query.
//
// URL parsing is deliberately not expressed through form tags. Generated
// Parse{Entity}Query functions own the wire contract, while Limit and Offset
// keep the Go-layer repair semantics used by direct callers.
type SortSpec struct {
	Key  string `json:"key"`
	Desc bool   `json:"desc,omitempty"`
}

type ListRequest struct {
	Size int        `json:"size,omitempty"`
	Page int        `json:"page,omitempty"`
	Sort []SortSpec `json:"sort,omitempty"`
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
