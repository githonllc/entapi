// Package entapi is the runtime half of the entapi code generator: the
// Layer 2 code a consumer's PRODUCTION binary links, and nothing else.
//
// It imports the standard library and nothing more. Not ent, not the ent
// codegen packages, not the source formatter, and — the reason this package
// exists at all (#15) — not the templates. Until the split, a consumer's binary
// that wanted ErrValidation got five embedded templates and a template loader
// running during package initialisation, because the sentinel and the loader
// were declared in the same package. TestRuntimePackageIsGeneratorFree, in the
// generator package, asserts the separation against `go list -deps` rather than
// against a reading of these import blocks.
//
// The package name is deliberately entapi rather than runtime: generated
// code and consumer call sites read entapi.ListPage, entapi.ErrValidation
// and entapi.WithSoftDeleted whichever path they arrive by, so the move cost
// exactly one line per file — the import — and no call site at all.
//
// # Import path
//
//	import "github.com/githonllc/entapi/runtime"
//
// The generation-time half — the annotation builders, SoftDeleteMixin and the
// ent extension — stays at github.com/githonllc/entapi and is imported by
// schema files and entc.go. A file needing both imports both, aliasing one.
//
// [ListPage] runs a filtered, ordered, paginated query against anything
// satisfying [Query] — in practice an ent query builder — and converts the
// results. [GetOne] fetches one entity by id. Both infer all their type
// arguments at the call site, including the identifier type:
//
//	user, err := entapi.GetOne(ctx, db.User.Get, NewUserResponse, id)     // ID is uuid.UUID
//	tag,  err := entapi.GetOne(ctx, db.Tag.Get, NewTagResponse, tagID)    // ID is int
//
//	page, err := entapi.ListPage(ctx, db.User.Query(),
//	    filter.Predicates(), orderOpts, req, NewUserResponse)
//
// [SaveOne] is the mutation half: it runs an ent create or update builder — both
// satisfy [Saver] — and converts the entity it returns. Generated wiring calls
// it with the builder the validated request already wrote itself onto, so a
// create or an update stays a single expression:
//
//	resp, err := entapi.SaveOne(ctx, v.Apply(db.User.Create()), NewUserResponse)
//
// [ListPage] uses offset pagination, which is O(n) deep, costs a COUNT per
// page, and can skip or repeat rows under concurrent writes. It is also the
// only pagination this package has: Cursor, PageInfo, EncodeCursor,
// DecodeCursor and ListRequest.Cursor have been removed, and generated
// {Entity}ListResponse no longer carries a PageInfo field — see the README for
// the migration note.
//
// [MaxPageSize] is the single place the page-size ceiling is decided, with a
// single reaction to crossing it: [ListRequest.Limit] clamps. Generated query
// parsers reject non-positive wire values before constructing a ListRequest;
// Limit and [ListRequest.Offset] retain their repair semantics for direct Go
// callers.
//
// A ListRequest zero value is usable as-is; there is deliberately no defaulting
// method, so there is none to forget. ListRequest.SetDefaults has been removed —
// see the README for the migration note.
//
// Query wire v2 removed the previous scalar sort fields, request validation
// method and sort-key helper. Callers regenerate a typed Parse{Entity}Query
// function and pass its []SortSpec to the generated {Entity}Order allow-list.
// AppendIf and AppendIfSlice likewise became
// [AppendEach] and [AppendEachSlice] when filter slots became slices. See the
// README migration notes for the old-to-new wire spelling table.
//
// # Binding, classifying and writing
//
// [BindJSON], [Status] and [WriteJSON] are the three steps every generated HTTP
// handler is built from, exported so a hand-written endpoint on any router can
// call them instead of copying a generated body.
//
//	func BindJSON(w http.ResponseWriter, r *http.Request, tags []string, dst any) error
//	func Status(err error, onValidation int) int
//	func WriteJSON(w http.ResponseWriter, status int, v any) error
//
// [BindJSON] applies the three bind rules that have no knobs: a 1 MiB
// http.MaxBytesReader cap, an application/json media-type check, and rejection
// of any body key absent from tags. Its error is total — every error it returns
// wraps exactly one of [ErrUnsupportedMediaType], [ErrRequestTooLarge] or
// [ErrValidation], so [Status] classifies all of them and there is no fourth
// case. It takes w only because http.MaxBytesReader needs it and writes nothing
// itself, so the caller owns the response and its shape.
//
// [Status] answers 415 and 413 for the two bind sentinels, 404 for [ErrNotFound],
// 409 for [ErrAlreadyExists], onValidation for [ErrValidation], 500 for anything
// else and 0 for a nil error. onValidation is the whole 400-vs-422 convention:
// generated handlers pass 400 for a bind failure and 422 for a middle-step
// failure — malformed versus well-formed-but-rejected.
//
// [WriteJSON] marshals before touching the response, so a marshal failure
// becomes a clean 500 problem response rather than a truncated 200.
//
// # Error mapping
//
// [ErrorMapper] translates a persistence layer's errors into [ErrNotFound] and
// [ErrAlreadyExists]. It takes predicates as function values so the runtime
// stays ent-free.
//
// Consumers do not construct one. Generation writes ent/entapi_errors.go,
// one declaration for the package, and every generated operation returns
// through it:
//
//	var ErrorMap = entapi.NewErrorMapper(IsNotFound, IsConstraintError)
//
// One variable rather than a per-call parameter is what makes IsNotFound answer
// the same way whichever operation failed.
//
// Uniqueness requires [ErrorMapper.WithUniqueViolation], installed on that
// variable: ent.IsConstraintError is true for a duplicate key and a foreign-key
// violation alike, so mapping it straight to ErrAlreadyExists would report the
// latter as the former. Until it is installed, nothing claims ErrAlreadyExists
// — not-found keeps working and already-exists is simply given up. What the
// mapper cannot classify it returns unchanged rather than guessing.
//
// # Soft delete
//
// [WithSoftDeleted] and [WithHardDelete] are the two context switches the
// generated traverser and hook read. They are separate keys on purpose: one
// changes what reads return, the other changes what a delete does, and neither
// implies the other. The schema-side half — SoftDeleteMixin — is in the
// generator package, because a schema is only ever loaded at generation time.
package entapi
