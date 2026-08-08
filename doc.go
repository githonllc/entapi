// Package entapi provides an [entgo.io/ent] extension that generates
// request/response DTOs, a query surface and operation wiring from annotated
// Ent schemas.
//
// This package is the code-generation half only. It is loaded by ent at
// `go generate` time and produces request/response DTOs, filter structs with a
// sort allow-list, and one free function per operation, for each Ent schema
// annotated with EntAPI markers. It also carries the annotation types and
// [SoftDeleteMixin], which consumer SCHEMAS embed — also generation-time code.
//
// The runtime half — the generic CRUD operations, ListRequest, the error
// sentinels, the error mapper and the pointer helpers — lives in
// [github.com/githonllc/entapi/runtime], which imports no ent package and
// embeds no templates.
//
// # Migration: the runtime moved (#15)
//
// The two halves used to share this package, so importing it for
// entapi.ErrValidation also embedded five templates into the consumer's
// binary and ran the template loader during package initialisation. The runtime
// symbols are GONE from this package; there are no deprecation aliases.
//
//	was: import "github.com/githonllc/entapi"
//	now: import "github.com/githonllc/entapi/runtime"
//
// The runtime package is still named entapi, so every `entapi.X` call
// site is unchanged — only the import path moves. Schema files, which use the
// annotation builders and [SoftDeleteMixin], keep importing this package and
// need no edit. A file that needs both imports both, aliasing one.
//
// Regenerating picks the new path up automatically:
// [WithEntAPIPackage]'s default is now the runtime path.
//
// # Quick Start
//
// Annotate fields in your Ent schema:
//
//	field.String("name").
//	    Annotations(entapi.DefaultField().
//	        WithRequired(entapi.ScopeCreate))
//
// Annotate edges with [Edge]. An edge's exposure is its own decision, not one
// derived from its foreign-key field: "put author_id in the response" and "put
// a nested author object in the response" are different intents, and a to-many
// edge has no field on this entity to derive anything from.
//
//	edge.To("posts", Post.Type).
//	    Annotations(entapi.Edge().InResponse())
//
// A field stays out of responses two ways: by not carrying [ScopeResponse], or
// by being marked Sensitive() in the ent schema. The second is unconditional —
// ent's fact overrides a response scope the annotation granted, for both
// {Entity}Response and {Entity}Summary — so passwords and secrets need no scope
// bookkeeping. [InputOnlyField] and custom scope lists remain the mechanism for
// withholding a field ent says nothing about. DomainField.Sensitive and
// AsSensitive have been removed; they were never read by anything, so a field
// marked sensitive was emitted into the response struct regardless, and that is
// the leak the ent fact closes. See the README for the migration note.
//
// Wire the extension in your entc.go:
//
//	func main() {
//	    ext := entapi.NewExtensionWithOptions(
//	        entapi.WithEntAPIPackage("github.com/githonllc/entapi/runtime"),
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
// Eleven of the twenty-six exported settings reach a template:
// [DomainField.Scopes], [DomainField.Required], [DomainEdge.Scopes],
// [DomainEdge.JSONKey], [DomainField.Filterable], [DomainField.Searchable],
// [DomainField.Sortable], and the [ScopeCreate], [ScopeUpdate], [ScopeQuery]
// and [ScopeResponse] constants. The other fifteen are the whole
// [FieldMetadata] block, which is accepted and stored but changes nothing that
// is generated yet.
//
// # Migration
//
// Generated RegisterSoftDelete has been removed. Regenerate and delete every
// ent.RegisterSoftDelete(client) call: embedding [SoftDeleteMixin] now injects
// the hook and interceptor into every generated client automatically. This
// also makes the soft-delete hook outermost, ahead of hooks added later with
// Client.Use. See the README migration notes.
//
// DomainField.Validation and WithValidation are gone, with no replacement:
// Validate() on the generated request types supersedes them. WithDescription
// and WithExample still exist and still chain, but store onto [FieldMetadata]
// rather than [DomainField] — read them as d.Metadata.Description and
// d.Metadata.Example. The entity-level DomainConfig annotation is gone
// entirely. See the README's migration sections for the before/after of each.
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
// See the README for the full annotation reference and generated code examples,
// and [github.com/githonllc/entapi/runtime] for what the generated code
// calls at run time.
package entapi
