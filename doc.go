// Package entdomain provides an [entgo.io/ent] extension that generates
// request/response DTOs and service/handler scaffolding from annotated Ent schemas.
//
// It serves two roles:
//
//   - At code-generation time (go generate), it produces request/response DTOs,
//     BaseService structs, and BaseHandler structs for each Ent schema
//     annotated with EntDomain markers.
//
//   - At runtime, it provides the types and helpers that the generated
//     code depends on: the generic CRUD operations [ListPage] and [GetOne],
//     [Page], [ListRequest], [PageInfo], error sentinel values, and pointer
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
// Wire the extension in your entc.go:
//
//	func main() {
//	    ext := entdomain.NewExtensionWithOptions(
//	        entdomain.WithEntDomainPackage("github.com/githonllc/entdomain"),
//	        entdomain.WithBaseService(true),
//	        entdomain.WithBaseHandler(true),
//	    )
//	    if err := entc.Generate("./schema", &gen.Config{}, entc.Extensions(ext)); err != nil {
//	        log.Fatal(err)
//	    }
//	}
//
// Run go generate to produce {entity}_dto.go, {entity}_base_service.go,
// and {entity}_base_handler.go for each annotated schema.
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
// [ListPage] uses offset pagination, which is O(n) deep, costs a COUNT per
// page, and can skip or repeat rows under concurrent writes.
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
