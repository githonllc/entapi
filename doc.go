// Package entdomain provides an [entgo.io/ent] extension that generates
// request/response DTOs, a query surface and operation wiring from annotated
// Ent schemas.
//
// It serves two roles:
//
//   - At code-generation time (go generate), it produces request/response DTOs,
//     filter structs with a sort allow-list, and one free function per
//     operation, for each Ent schema annotated with EntDomain markers.
//
//   - At runtime, it provides the types and helpers that the generated
//     code depends on: the generic CRUD operations [ListPage], [GetOne] and
//     [SaveOne], [Page], [ListRequest], error sentinel values, and pointer
//     utilities.
//
// The runtime half imports no ent package. Its entity-specific parts arrive as
// type parameters and function values supplied by generated wiring, so a
// consumer's binary does not link the generator's dependency graph and the
// identifier type is not hardcoded.
//
// # Quick Start
//
// Annotate fields in your Ent schema:
//
//	field.String("name").
//	    Annotations(entdomain.DefaultField().
//	        WithRequired(entdomain.ScopeCreate))
//
// Annotate edges with [Edge]. An edge's exposure is its own decision, not one
// derived from its foreign-key field: "put author_id in the response" and "put
// a nested author object in the response" are different intents, and a to-many
// edge has no field on this entity to derive anything from.
//
//	edge.To("posts", Post.Type).
//	    Annotations(entdomain.Edge().InResponse())
//
// A field stays out of responses by not carrying [ScopeResponse] — that is the
// only mechanism, so use [InputOnlyField] or a custom scope list for passwords
// and secrets. DomainField.Sensitive and AsSensitive have been removed; they
// were never read by anything, so a field marked sensitive was emitted into the
// response struct regardless. See the README for the migration note.
//
// Wire the extension in your entc.go:
//
//	func main() {
//	    ext := entdomain.NewExtensionWithOptions(
//	        entdomain.WithEntDomainPackage("github.com/githonllc/entdomain"),
//	    )
//	    if err := entc.Generate("./schema", &gen.Config{}, entc.Extensions(ext)); err != nil {
//	        log.Fatal(err)
//	    }
//	}
//
// Run go generate to produce {entity}_dto.go, {entity}_filter.go and
// {entity}_wiring.go for each annotated schema.
//
// The generated base service and base handler have been removed, along with
// WithBaseService and WithBaseHandler — see the README for the migration note.
// Every artifact is now emitted unconditionally for an annotated entity, so
// there is nothing left to switch on.
//
// # Supported identifier types
//
// All of them. The identifier is rendered from the schema's own type in every
// template and arrives at the runtime as a type parameter, so an int, a string
// and a uuid.UUID primary key are equally supported. The uuid.UUID restriction
// that used to be enforced at generation time belonged to the base service and
// base handler templates and went with them.
//
// # What the annotations actually do
//
// Eleven of the twenty-seven exported settings reach a template:
// [DomainField.Scopes], [DomainField.Required], [DomainEdge.Scopes],
// [DomainEdge.JSONKey], [DomainField.Filterable], [DomainField.Searchable],
// [DomainField.Sortable], and the [ScopeCreate], [ScopeUpdate], [ScopeQuery]
// and [ScopeResponse] constants. Every other one — the whole [FieldMetadata]
// block, [DomainField.Validation], [DomainField.Description] and
// [DomainField.Example] — is accepted and stored but changes nothing that is
// generated yet.
//
// The three query markers are opt-in per field: no preset builder grants them,
// because they now produce real query parameters and a real sort allow-list,
// and a permissive default would make essentially every response-visible field
// orderable. [ScopeQuery] is the gate the markers sit behind — it says the
// field may be reached from the query API, and the marker says in which
// dimension.
//
// The README's "Annotation surface" section lists each with the issue that will
// consume it. That list is derived by a test, not maintained by hand, so it
// cannot drift from the code in either direction.
//
// # Runtime
//
// [ListPage] runs a filtered, ordered, paginated query against anything
// satisfying [Query] — in practice an ent query builder — and converts the
// results. [GetOne] fetches one entity by id. Both infer all their type
// arguments at the call site, including the identifier type:
//
//	user, err := entdomain.GetOne(ctx, db.User.Get, NewUserResponse, id)     // ID is uuid.UUID
//	tag,  err := entdomain.GetOne(ctx, db.Tag.Get, NewTagResponse, tagID)    // ID is int
//
//	page, err := entdomain.ListPage(ctx, db.User.Query(),
//	    filter.Predicates(), orderOpts, req, NewUserResponse)
//
// [SaveOne] is the mutation half: it runs an ent create or update builder — both
// satisfy [Saver] — and converts the entity it returns. Generated wiring calls
// it with the builder the validated request already wrote itself onto, so a
// create or an update stays a single expression:
//
//	resp, err := entdomain.SaveOne(ctx, v.Apply(db.User.Create()), NewUserResponse)
//
// [ListPage] uses offset pagination, which is O(n) deep, costs a COUNT per
// page, and can skip or repeat rows under concurrent writes. It is also the
// only pagination this package has: Cursor, PageInfo, EncodeCursor,
// DecodeCursor and ListRequest.Cursor have been removed, and generated
// {Entity}ListResponse no longer carries a PageInfo field — see the README for
// the migration note.
//
// [MaxPageSize] is the single place the page-size ceiling is decided, with a
// single reaction to crossing it: [ListRequest.Limit] clamps. [ListRequest.Validate]
// says nothing about Size or Page, because Limit and [ListRequest.Offset] sit on
// the only path into ListPage and apply whether or not anyone validates. The
// validate struct tag on ListRequest.Size carries no numeric ceiling of its own,
// because a struct tag cannot reference a constant and so can only drift away
// from one.
//
// A ListRequest zero value is usable as-is; there is deliberately no defaulting
// method, so there is none to forget. ListRequest.SetDefaults has been removed —
// see the README for the migration note.
//
// [ListRequest.Validate] is left with Order, compared case-insensitively to
// match [ListRequest.SortKey], which is what actually decides the direction.
//
// # Error mapping
//
// [ErrorMapper] translates a persistence layer's errors into [ErrNotFound] and
// [ErrAlreadyExists]. It takes predicates as function values so the runtime
// stays ent-free:
//
//	var mapper = entdomain.NewErrorMapper(ent.IsNotFound, ent.IsConstraintError)
//
// Uniqueness requires [ErrorMapper.WithUniqueViolation]: ent.IsConstraintError
// is true for a duplicate key and a foreign-key violation alike, so mapping it
// straight to ErrAlreadyExists would report the latter as the former. What the
// mapper cannot classify it returns unchanged rather than guessing.
//
// See the README for the full annotation reference and generated code examples.
package entdomain
