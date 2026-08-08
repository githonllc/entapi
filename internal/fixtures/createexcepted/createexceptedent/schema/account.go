package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"

	"github.com/githonllc/entapi/api"
)

// Account cannot be created through its derived request, and explicitly
// excludes create so the entire broken family is absent.
type Account struct{ ent.Schema }

func (Account) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
		field.String("secret").Annotations(api.Hidden()),
	}
}

func (Account) Annotations() []schema.Annotation {
	return []schema.Annotation{api.Resource().Except(api.OpCreate)}
}
