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
//	Draft   hard delete, owns a UNIQUE + REQUIRED  -> the #100 shape: a required
//	        api.Expand()ed edge to Doc                edge whose target can vanish
//	                                                  from the read side
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

// Draft is the shape #100 was reported against, transplanted onto this fixture:
// a hard-delete entity whose edge to a soft-deletable target is Unique,
// Required and api.Expand()ed. The reporter's pair was Session -> User.
//
// The schema says every Draft has a Doc, and the foreign key still says so
// after the Doc is soft-deleted — the row is on disk and the constraint holds.
// Only the READ side changes: the traverser removes the Doc from the eager-load
// sub-query, so the edge comes back loaded-and-absent and the response carries
// `"doc": null`. Soft delete does not cascade, so the Draft itself stays in
// every list. Both facts are asserted against a real database in
// internal/softdeleteproof.
type Draft struct {
	ent.Schema
}

// Fields of the Draft.
func (Draft) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),

		field.String("headline"),
	}
}

// Edges of the Draft.
func (Draft) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("doc", Doc.Type).
			Unique().
			Required().
			Annotations(api.Expand()),
	}
}

// Draft excepts the write families for the same reason the consumer's Session
// does: its required "doc" edge declares no edge.Field(), so the foreign key is
// not an ent field, so nothing puts a setter for it in a create request and the
// generated CreateDraft could never succeed. Since #110 generation REFUSES
// exactly that shape (requiredEdgeWithoutFieldConflicts in schema_conflicts.go,
// with internal/fixtures/requirededge as its own fixture), so the Except here
// is no longer only tidiness — it is what keeps this fixture generating at all,
// and it keeps the fixture about #100 rather than about the refused shape.
func (Draft) Annotations() []schema.Annotation {
	return []schema.Annotation{api.Resource().Except(api.OpCreate, api.OpPatch)}
}
