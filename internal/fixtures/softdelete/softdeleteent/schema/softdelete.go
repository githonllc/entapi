// Package schema holds the hand-written ent schemas for the "softdelete"
// codegen fixture: the three shapes the soft-delete traverser generation has to
// tell apart (#18).
//
//	Doc     embeds entapi.SoftDeleteMixin      -> soft-deletable
//	Note    embeds nothing, owns Docs by an edge  -> hard delete, and the
//	                                                 eager-load path a filtered
//	                                                 sub-query has to reach
//	Ledger  declares a deleted_at field BY HAND,  -> NOT soft-deletable
//	        Optional but not Nillable, with no
//	        mixin
//
// Ledger is the fixture obligation transferred from #12, and it is what pins
// the decision on the naming convention: before #18 an entity was soft-deletable
// iff it happened to carry a Nillable time.Time literally named "deleted_at".
// Ledger's field is named exactly that and is deliberately not Nillable, so
// under the old rule it was one modifier away from silently acquiring
// row-level filtering it never asked for. Under the annotation rule it is not
// soft-deletable for a stated reason — it does not embed the mixin — and the
// modifier is irrelevant.
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/privacy"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entapi"
	"github.com/githonllc/entapi/api"
)

// Doc is the conforming soft-deletable entity.
type Doc struct {
	ent.Schema
}

// Mixin of the Doc. This one line is the whole opt-in.
func (Doc) Mixin() []ent.Mixin {
	return []ent.Mixin{
		entapi.SoftDeleteMixin{},
	}
}

// Policy denies every undecided Doc operation. Tests opt in through an allow
// decision and prove the soft-delete hook's re-dispatched update preserves it.
func (Doc) Policy() ent.Policy {
	return privacy.Policy{
		Query:    privacy.QueryPolicy{privacy.AlwaysDenyRule()},
		Mutation: privacy.MutationPolicy{privacy.AlwaysDenyRule()},
	}
}

// Fields of the Doc.
func (Doc) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),

		field.String("title"),
	}
}

// Edges of the Doc.
func (Doc) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("note", Note.Type).
			Ref("docs").
			Unique().
			Annotations(api.Expand()),
	}
}

func (Doc) Annotations() []schema.Annotation { return []schema.Annotation{api.Resource()} }

// Note is the hard-delete entity. Nothing about it may change because Doc is
// soft-deletable, and its "docs" edge is the eager-load path: a sub-query built
// for WithDocs has to be filtered too, or a deleted Doc reappears through its
// parent.
type Note struct {
	ent.Schema
}

// Fields of the Note.
func (Note) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),

		field.String("body"),
	}
}

// Edges of the Note.
func (Note) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("docs", Doc.Type).
			Annotations(api.Expand()),
	}
}

func (Note) Annotations() []schema.Annotation { return []schema.Annotation{api.Resource()} }

// Ledger carries a hand-written deleted_at that is NOT Nillable and no mixin.
// It must be treated as an ordinary hard-delete entity.
type Ledger struct {
	ent.Schema
}

// Fields of the Ledger.
func (Ledger) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),

		field.String("entry"),

		// Named exactly like the retired convention's trigger, Optional so ent
		// still generates a DeletedAtIsNil predicate for it — everything the
		// old rule needed except Nillable. It is tracked by ent and appears in
		// no HTTP struct, so it carries no entapi annotation.
		field.Time("deleted_at").
			Optional().
			Annotations(api.Hidden()),
	}
}

func (Ledger) Annotations() []schema.Annotation { return []schema.Annotation{api.Resource()} }
