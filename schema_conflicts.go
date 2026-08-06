package entdomain

import (
	"fmt"
	"strings"

	"entgo.io/ent/entc/gen"
)

// checkGraphConflicts rejects a graph this package cannot generate correct code
// for — either because an entdomain annotation contradicts the ent schema it is
// attached to, or because the schema uses a shape the templates cannot express.
//
// The policy, decided on #10 and applying to every later generation slice:
//
//	An annotation that contradicts the ent schema fails generation, reporting
//	both facts. Anything that can be generated correctly is generated, not
//	refused.
//
// It runs before ent's own generator, so a rejected schema leaves nothing on
// disk — no half-written package, no stale file from the previous run.
//
// The whole graph is checked before anything is reported, so a schema author
// sees every problem at once rather than one per `go generate` cycle. Both
// classes share one list for that reason: they are one answer to "why did
// generation stop", and each line names its own subject.
//
// cfg decides which checks apply, because not every refusal is unconditional:
// the identifier-type check is a property of the base service and base handler
// templates, which are opt-in. A DTO-only generation is correct for any
// identifier type and must not be refused. A nil cfg applies only the
// unconditional checks.
func checkGraphConflicts(g *gen.Graph, cfg *ExtensionConfig) error {
	emitsIDSignatures := cfg != nil && (cfg.GenerateBaseService || cfg.GenerateBaseHandler)

	var conflicts []string
	for _, node := range g.Nodes {
		if len(domainFields(node)) == 0 {
			// Generation skips this node entirely, so nothing it declares can
			// produce output to be wrong about.
			continue
		}
		conflicts = append(conflicts, nodeConflicts(node)...)
		if emitsIDSignatures {
			if msg := unsupportedIDType(node); msg != "" {
				conflicts = append(conflicts, msg)
			}
		}
	}
	if len(conflicts) == 0 {
		return nil
	}
	return fmt.Errorf("entdomain: %d schema problem(s) prevent generation:\n  - %s",
		len(conflicts), strings.Join(conflicts, "\n  - "))
}

// supportedIDType is the one identifier type the generated base service and
// base handler can express, spelled exactly as the templates emit it.
//
// It is a single constant rather than a set because the templates hardcode the
// type: base_service.tmpl and base_handler.tmpl write "uuid.UUID" into every
// hook signature, every CRUD method and the cursor round-trip. Widening the set
// means teaching the templates the entity's own id type, which is #29.
const supportedIDType = "uuid.UUID"

// unsupportedIDType reports an entity whose primary key the generated base
// service and base handler cannot be written against, or "" when there is none.
//
// The identifier type is not a matter of taste here. base_service.tmpl declares
// GetByID, Update, Delete, DeleteBatch and both delete hooks in terms of
// uuid.UUID, and base_handler.tmpl repeats it. Against an int-keyed entity ent
// generates Get(ctx, int), so the emitted file fails to compile in seven places
// — inside the consumer's own ent package, naming ent's methods, with nothing
// pointing back at the annotation that asked for it. Refusing here is the only
// place the cause is still visible.
//
// The comparison is against the rendered type name rather than the ent type
// constant, because the rendered name is exactly what the template writes: an
// id declared with a GoType of its own is a distinct Go type even when ent
// still classifies it as a UUID, and the generated signatures would not accept
// it.
func unsupportedIDType(node *gen.Type) string {
	if node.ID == nil || node.ID.Type == nil {
		return ""
	}
	actual := node.ID.Type.String()
	if actual == supportedIDType {
		return ""
	}
	return fmt.Sprintf(
		"%s.%s: primary key is of type %q, but the generated base service and base handler declare every identifier as %s "+
			"(templates/base_service.tmpl, templates/base_handler.tmpl), so the emitted code would not compile against this entity. "+
			"entdomain generates a base service for %s primary keys only (see issue #29); give %s a field.UUID(%q, uuid.UUID{}) primary key, "+
			"or generate this schema without WithBaseService/WithBaseHandler",
		node.Name, node.ID.Name, actual, supportedIDType,
		supportedIDType, node.Name, node.ID.Name,
	)
}

// nodeConflicts returns one human-readable message per contradiction found on
// one entity — its fields first, then its edges. Each message names the entity,
// the field or edge, and both conflicting facts, because the schema author has
// to know which of the two to change.
func nodeConflicts(node *gen.Type) []string {
	var out []string
	for _, f := range node.Fields {
		if getDomainFieldAnnotation(f) == nil {
			continue
		}
		if f.Immutable && hasDomainScope(f, ScopeUpdate) {
			out = append(out, immutableUpdateConflict(node, f))
		}
		out = append(out, queryConflicts(node, f)...)
	}
	out = append(out, asymmetricSelfEdgeConflicts(node)...)
	return out
}

// asymmetricSelfEdgeConflicts reports every self-referential edge pair on this
// entity whose two ends disagree about whether they carry a DomainEdge
// annotation at all.
//
// The pair is found from the inverse end: gen resolves Edge.Ref to the assoc
// edge it names, and a self-referential pair is one whose target is the entity
// that owns it, so both ends sit in the same Edges() slice and the author has
// them both in view.
//
// Only "annotated" versus "not annotated at all" counts. Two ends carrying
// different annotations are two decisions the author wrote down, and this
// package has no standing to overrule them; two bare ends are one decision, not
// to expose the relationship. It is the mixed case that has no reading as
// intent, because the chained declaration produces exactly it by accident.
func asymmetricSelfEdgeConflicts(node *gen.Type) []string {
	var out []string
	for _, inverse := range node.Edges {
		if !inverse.IsInverse() || inverse.Ref == nil {
			continue
		}
		if inverse.Type == nil || inverse.Type.Name != node.Name {
			// Not self-referential: the assoc end lives on another entity, is
			// annotated by another author on another day, and exposing one
			// direction only is ordinary there rather than suspicious.
			continue
		}
		assoc := inverse.Ref
		assocAnnotated := getDomainEdgeAnnotation(assoc) != nil
		inverseAnnotated := getDomainEdgeAnnotation(inverse) != nil
		if assocAnnotated == inverseAnnotated {
			continue
		}
		out = append(out, asymmetricSelfEdgeConflict(node, assoc, inverse, assocAnnotated))
	}
	return out
}

// asymmetricSelfEdgeConflict describes one self-referential pair annotated on
// one end only.
//
// The trap this exists for: written as
//
//	edge.To("children", Tree.Type).From("parent").Field("parent_id").Annotations(a)
//
// ent hands Annotations() to the inverse builder, so the annotation lands on
// "parent" and the assoc edge "children" is left bare — and a bare edge is
// precisely how a schema author says "do not expose this". Generation therefore
// drops "children" from the response and the eager-load plan with nothing to
// report, and the author is left looking at a missing relation. #22 lost real
// time to it before declaring the pair as two edges instead.
//
// The two spellings are indistinguishable by the time gen sees them — the
// chained form's Descriptor.Ref does not survive into gen.Edge — so the check
// is on the asymmetry rather than on the syntax, and the message names the
// chained form as the likely cause instead of asserting it.
//
// One end exposed on purpose stays expressible: annotate the other end with a
// bare entdomain.Edge(). It grants no scope, so nothing about the output
// changes; what changes is that the decision is written down where the next
// reader, and this check, can see it.
func asymmetricSelfEdgeConflict(node *gen.Type, assoc, inverse *gen.Edge, assocAnnotated bool) string {
	annotated, bare := inverse, assoc
	cause := fmt.Sprintf(
		"declaring the pair in the chained form edge.To(%q, %s.Type).From(%q)...Annotations(...) produces exactly this, "+
			"because ent gives Annotations() to the inverse builder and leaves the assoc edge %q bare",
		assoc.Name, node.Name, inverse.Name, assoc.Name)
	if assocAnnotated {
		annotated, bare = assoc, inverse
		cause = fmt.Sprintf("the %q end was most likely annotated and the %q end forgotten", assoc.Name, inverse.Name)
	}

	carries := "carries a DomainEdge annotation"
	if hasEdgeScope(annotated, ScopeResponse) {
		carries = "is annotated for the response"
	}

	return fmt.Sprintf(
		"%s.%s / %s.%s: the two ends of this self-referential edge pair disagree — %s.%s %s while %s.%s carries no DomainEdge annotation at all, "+
			"so %q appears in no response type and in no eager-load plan, and nothing else in generation says so; %s. "+
			"Declare the two ends separately and give each its own annotation — edge.To(%q, %s.Type).Annotations(entdomain.Edge().InResponse()) "+
			"and edge.From(%q, %s.Type).Ref(%q).Annotations(entdomain.Edge().InResponse()) — "+
			"or, to expose %q alone on purpose, annotate %q with a bare entdomain.Edge(), which grants no scope and says the end was considered",
		node.Name, assoc.Name, node.Name, inverse.Name,
		node.Name, annotated.Name, carries, node.Name, bare.Name,
		bare.Name, cause,
		assoc.Name, node.Name,
		inverse.Name, node.Name, assoc.Name,
		annotated.Name, bare.Name,
	)
}

// queryConflicts reports the contradictions between a field's query markers and
// what ent generates for that field.
//
// All four have the same shape as the immutable/update conflict above: the
// annotation asks for a call to something ent never wrote. Emitting it anyway
// produces an undefined symbol inside the consumer's own ent package, naming
// ent's API, with nothing pointing back at the annotation that asked for it.
// Silently dropping the marker instead would be worse — the parameter would
// vanish from the query API without a word, and the caller would find out by
// getting unfiltered results.
func queryConflicts(node *gen.Type, f *gen.Field) []string {
	a := getDomainFieldAnnotation(f)
	if a == nil {
		return nil
	}
	var out []string

	if (a.Filterable || a.Searchable || a.Sortable) && !hasDomainScope(f, ScopeQuery) {
		out = append(out, fmt.Sprintf(
			"%s.%s: annotation marks the field %s but withholds scope %q, so it is not exposed to the query API and no query artifact is generated for it; "+
				"add %s to the field's scopes (entdomain.DefaultField(), CreateOnlyField() and OutputOnlyField() all carry it), or drop the marker",
			node.Name, f.Name, markerList(a), ScopeQuery, ScopeQuery,
		))
	}

	if a.Searchable && !fieldHasOp(f, gen.Contains) {
		out = append(out, fmt.Sprintf(
			"%s.%s: annotation marks the field Searchable, but ent derives no Contains predicate for type %q "+
				"(entc/gen/func.go fieldOps), so there is no %sContains to put in the free-text disjunction. "+
				"Free-text search is a substring match and only string fields have one — drop AsSearchable(), or use AsFilterable() for exact matching",
			node.Name, f.Name, f.Type.String(), f.StructField(),
		))
	}

	if a.Filterable && len(f.Ops()) == 0 {
		out = append(out, fmt.Sprintf(
			"%s.%s: annotation marks the field Filterable, but ent derives no predicates at all for type %q "+
				"(entc/gen/func.go fieldOps), so the filter group would be empty and the parameter would silently do nothing. Drop AsFilterable()",
			node.Name, f.Name, f.Type.String(),
		))
	}

	if a.Sortable && (f.Type == nil || !f.Type.Comparable()) {
		out = append(out, fmt.Sprintf(
			"%s.%s: annotation marks the field Sortable, but type %q is not comparable, so ent's order builders skip it "+
				"(entc/gen/template/dialect/sql/meta.tmpl) and there is no %s to put in the sort allow-list. Drop AsSortable()",
			node.Name, f.Name, f.Type.String(), f.OrderName(),
		))
	}

	return out
}

// markerList names the query markers a field carries, so the scope message says
// which annotation the author has to reconcile.
func markerList(a *DomainField) string {
	var set []string
	if a.Filterable {
		set = append(set, "Filterable")
	}
	if a.Searchable {
		set = append(set, "Searchable")
	}
	if a.Sortable {
		set = append(set, "Sortable")
	}
	return strings.Join(set, "/")
}

// fieldHasOp reports whether ent derived op for this field. The question is
// always asked of ent's own table rather than of a copy of it — that table is
// what where.go is generated from, so a second one could only disagree.
func fieldHasOp(f *gen.Field, want gen.Op) bool {
	for _, op := range f.Ops() {
		if op == want {
			return true
		}
	}
	return false
}

// immutableUpdateConflict describes the field-level contradiction: a field ent
// marks Immutable that carries ScopeUpdate.
//
// ent's Update and UpdateOne builders iterate MutableFields, which excludes
// immutable fields, so no update setter exists for one. Generating the update
// request anyway produces a call to a method that is not there, and the
// consumer discovers it as a compile error in their own ent package with no
// indication of the cause. Silently dropping the field instead would be worse:
// the field would vanish from the PATCH API without a word, and neither
// encoding/json nor the generated Validate can observe a key that has no struct
// field to land in — an API client would find out in production.
//
// Note that entdomain.DefaultField() grants ScopeUpdate, so an immutable field
// carrying the default annotation always lands here. That is intended: the fix
// is one annotation, and it has to be written down somewhere.
func immutableUpdateConflict(node *gen.Type, f *gen.Field) string {
	return fmt.Sprintf(
		"%s.%s: field is Immutable() in the ent schema but its DomainField annotation carries scope %q; "+
			"ent generates no Set%s on %sUpdateOne (update builders iterate MutableFields), so no update request can be generated for it. "+
			"Give the field an annotation without ScopeUpdate (entdomain.CreateOnlyField() or entdomain.OutputOnlyField()), or drop Immutable() from the field",
		node.Name, f.Name, ScopeUpdate, f.StructField(), node.Name,
	)
}
