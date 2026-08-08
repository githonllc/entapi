package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entapi/api"
)

// Secret has no response-scoped field at all: every annotated field is
// Sensitive, and gen.Type.Fields excludes the id, so responseFields is empty.
//
// This is the shape transferred onto #25 from #9. The v1 dto.tmpl guarded the
// response struct on responseFields being non-empty but emitted ListResponse
// outside that guard, so this entity produced a ListResponse referring to a
// Response type that did not exist. The response type is meaningful with only
// its ID field, so it is always emitted.
type Secret struct {
	ent.Schema
}

// Fields of the Secret.
func (Secret) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),

		field.String("token").Sensitive(),
	}
}

func (Secret) Annotations() []schema.Annotation { return []schema.Annotation{api.Resource()} }
