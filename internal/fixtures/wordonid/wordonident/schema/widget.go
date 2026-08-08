package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entapi/api"
)

type Widget struct{ ent.Schema }

func (Widget) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Annotations(api.Sortable()),
		field.String("name"),
	}
}

func (Widget) Annotations() []schema.Annotation { return []schema.Annotation{api.Resource()} }
