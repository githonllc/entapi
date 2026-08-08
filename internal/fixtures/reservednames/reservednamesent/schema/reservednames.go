// Package schema holds the hand-written ent schemas for the "reservednames"
// codegen fixture (#62): a schema whose entity NAME collides with a symbol this
// extension generates.
//
//	ErrorMap  an ordinary annotated entity that happens to be called ErrorMap.
//	          ent generates `type ErrorMap` for it; templates/errors.tmpl
//	          generates `var ErrorMap` into entapi_errors.go for any schema
//	          with an annotated entity. Both land in the consumer's one package,
//	          so the two declarations are `redeclared in this block` — a compile
//	          error inside the consumer's own repository with nothing pointing
//	          back at this schema. Generation is therefore refused.
//	Probe     the ordinary annotated entity that makes the refusal reachable:
//	          entapi_errors.go is only emitted when SOMETHING is annotated,
//	          so an ErrorMap entity alone would prove nothing.
//
// Probe does double duty. It is also the probe entity of
// TestDerivedEntityNamesMatchTheTemplates, which renders all five templates
// over this graph and compares the exported declarations they emit against
// derivedEntityNames. That is why it carries a create-scoped field, an
// update-scoped field, an output-only field, all three query markers AND the
// soft-delete mixin: every conditional emission in the five templates has to
// fire, or the guard's reverse direction would pass vacuously. Keep it that
// way — a field removed here silently narrows that guard.
//
// The mixin has a second job in this fixture: it is what makes softDeleteTypes
// non-empty, which is the condition under which RegisterSoftDelete — the other
// reserved graph-level name — is generated.
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entapi"
)

// ErrorMap is the colliding entity. Nothing about it is unusual except its
// name, which is the entire point: the schema is legal ent and legal entapi
// field-by-field, and only the generated package's identifier namespace says
// no.
type ErrorMap struct {
	ent.Schema
}

// Fields of the ErrorMap.
func (ErrorMap) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Annotations(entapi.IdField()),

		field.String("label").
			Annotations(entapi.DefaultField().WithRequired(entapi.ScopeCreate)),
	}
}

// Probe is the ordinary annotated entity. See the package comment for the two
// jobs it holds down.
type Probe struct {
	ent.Schema
}

// Mixin of the Probe: what makes the graph soft-deletable, and so what makes
// RegisterSoftDelete a generated name in this graph.
func (Probe) Mixin() []ent.Mixin {
	return []ent.Mixin{
		entapi.SoftDeleteMixin{},
	}
}

// Fields of the Probe.
func (Probe) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Annotations(entapi.IdField()),

		// create + update + response + the full query surface. Create and
		// update scopes are what make ProbeCreateRequest, ProbePatchRequest,
		// CreateProbe and UpdateProbe reachable at all.
		field.String("name").
			Annotations(entapi.DefaultField().
				WithRequired(entapi.ScopeCreate).
				AsFilterable().
				AsSearchable().
				AsSortable()),

		// response only, and sortable — the paging key.
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Annotations(entapi.OutputOnlyField().AsSortable()),
	}
}
