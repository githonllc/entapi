// Package entapi provides the code-generation half of EntAPI.
//
// Consumer entc programs import this package for [Extension]. Ent schema files
// import [github.com/githonllc/entapi/api] for schema-time annotations. Generated
// production code imports [github.com/githonllc/entapi/runtime], which depends
// only on the standard library and embeds no templates.
//
// # Quick Start
//
// Opt an entity into generation with api.Resource. Field shape is otherwise
// derived from Ent's Optional, Default, Nillable, Immutable and Sensitive
// declarations; annotations only express deviations.
//
//	func (Article) Annotations() []schema.Annotation {
//		return []schema.Annotation{api.Resource()}
//	}
//
//	func (Article) Fields() []ent.Field {
//		return []ent.Field{
//			field.String("title").
//				Annotations(api.Searchable(), api.Filterable(), api.Sortable()),
//			field.String("password_hash").Sensitive(),
//			field.Time("created_at").Default(time.Now).Immutable().
//				Annotations(api.ReadOnly()),
//		}
//	}
//
// api.Hidden and api.ReadOnly remove fields from request surfaces. Ent's
// Sensitive removes a field from responses while leaving it settable. Query
// dimensions are opt-in through api.Searchable, api.Filterable and api.Sortable.
// An edge carrying api.Expand appears one level deep as the target's Summary;
// JSONKey changes its response key.
//
//	edge.To("posts", Post.Type).
//		Annotations(api.Expand().JSONKey("articles"))
//
// api.Resource().Except(api.OpCreate, api.OpDelete) removes those public
// operation surfaces. Request DTOs and wiring functions remain available to
// service code, except that an unusable create family is omitted when create
// is explicitly excepted.
//
// Wire the extension into entc and regenerate:
//
//	func main() {
//		ext := entapi.NewExtensionWithOptions()
//		if err := entc.Generate("./schema", &gen.Config{}, entc.Extensions(ext)); err != nil {
//			log.Fatal(err)
//		}
//	}
//
// # Migration from the scope model
//
// The old field model was removed without compatibility aliases. Migrate by
// effect, not by constructor name:
//
//	DefaultField()                         -> no field annotation
//	InputOnlyField()                       -> Ent Sensitive()
//	OutputOnlyField()                      -> api.ReadOnly()
//	CreateOnlyField()                      -> Ent Immutable()
//	IdField()                              -> no annotation; Ent's ID is automatic
//	AuditLogField()                        -> api.ReadOnly()
//	NewDomainField()                       -> api.Hidden()
//	DomainFieldWithScopes(...)             -> spell the intended effect with Ent plus the five words
//	ScopeCreate / ScopeUpdate              -> derived from Optional, Default, Nillable and Immutable
//	ScopeResponse                          -> derived; use Hidden or Sensitive to remove it
//	ScopeQuery                             -> api.Searchable(), api.Filterable() and/or api.Sortable()
//	WithRequired(ScopeCreate)              -> no successor; requiredness is !Optional && !Default
//	AsSearchable/AsFilterable/AsSortable   -> api.Searchable/api.Filterable/api.Sortable
//	AsReadOnly                             -> api.ReadOnly()
//	AsWriteOnly                            -> Ent Sensitive()
//	WithMetadata and metadata builders     -> no successor
//	Edge().InResponse().As("key")          -> api.Expand().JSONKey("key")
//
// InputOnlyField previously expressed an HTTP-only promise. Sensitive is an
// Ent field-builder fact: it also binds service-layer and logging behaviour.
// That broader meaning is deliberate; an annotation cannot honestly alias it.
// WithRequired has no successor because HTTP requiredness no longer overrides
// the database schema.
//
// Generated Update{Entity} was renamed to Patch{Entity}. The generated base
// service, base handler and RegisterSoftDelete entry point were removed in
// earlier migrations. See README.md for the complete generated surface and
// refusal matrix.
package entapi
