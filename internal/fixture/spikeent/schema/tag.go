package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/githonllc/entapi"
)

// Tag exists for one reason: its primary key is not a UUID.
//
// Declaring no id field leaves ent's default, an autoincrementing int. User,
// Post and Category are all UUID-keyed, so without this schema the runtime's
// ID type parameter would only ever be instantiated one way — which is exactly
// how base_service.tmpl's hardcoded uuid.UUID went unnoticed. The identifier
// type is a property of the schema, and the runtime must not have an opinion
// about it.
type Tag struct {
	ent.Schema
}

func (Tag) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			Unique().
			Annotations(entapi.DefaultField().AsFilterable().AsSortable()),
	}
}
