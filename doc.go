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
// For the complete generated surface, the annotation model and the refusal
// matrix, see docs/GUIDE.md.
package entapi
