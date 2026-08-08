package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"

	"github.com/githonllc/entapi/api"
)

type Post struct{ ent.Schema }

func (Post) Fields() []ent.Field { return []ent.Field{field.String("title")} }

func (Post) Edges() []ent.Edge {
	return []ent.Edge{edge.To("authors", Author.Type).Annotations(api.Expand())}
}

func (Post) Annotations() []schema.Annotation { return []schema.Annotation{api.Resource()} }

// Author deliberately has no Resource annotation.
type Author struct{ ent.Schema }

func (Author) Fields() []ent.Field { return []ent.Field{field.String("name")} }
