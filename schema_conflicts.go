package entapi

import (
	"fmt"
	"strings"

	"entgo.io/ent/entc/gen"
)

// checkGraphConflicts rejects a graph this package cannot generate correct code
// for — either because an entapi annotation contradicts the ent schema it is
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
// Every check here is unconditional, because every artifact is now generated
// unconditionally. This function used to take the extension config as well, to
// decide whether the identifier-type refusal applied: base_service.tmpl and
// base_handler.tmpl wrote "uuid.UUID" into every signature, so an int-keyed
// entity had no correct output. Those templates are gone (#29) and the
// remaining ones render the id through $.ID.Type, so there is no identifier
// this package refuses and no configuration left to consult.
func checkGraphConflicts(g *gen.Graph) error {
	var conflicts []string
	for _, node := range g.Nodes {
		// Checked before the domain-field gate below: soft delete is declared
		// on the ent schema, not on the HTTP surface, so an entity with no
		// annotated field at all can still carry the mixin and still be
		// generated for.
		if msg := unusableSoftDeleteField(node); msg != "" {
			conflicts = append(conflicts, msg)
		}
		if len(domainFields(node)) == 0 {
			// Generation skips this node entirely, so nothing it declares can
			// produce output to be wrong about.
			continue
		}
		conflicts = append(conflicts, nodeConflicts(node)...)
	}
	// Outside the loop on purpose: this one is a property of the graph, not of
	// any one entity, and it has to see the UNannotated nodes too — ent
	// generates a type for those as well, and a type is all it takes to collide.
	conflicts = append(conflicts, reservedNameConflicts(g)...)
	if len(conflicts) == 0 {
		return nil
	}
	return fmt.Errorf("entapi: %d schema problem(s) prevent generation:\n  - %s",
		len(conflicts), strings.Join(conflicts, "\n  - "))
}

// unusableSoftDeleteField reports an entity whose soft-delete marker names a
// tombstone column the traverser cannot be written against, or "" when there is
// none.
//
// Two ways that happens, and neither is reachable by embedding
// entapi.SoftDeleteMixin as documented — both mean the annotation was
// attached by hand or by another mixin:
//
//   - the named field is not on the entity, so there is no column to filter on;
//   - the field is not Optional, so ent generates no <Field>IsNil predicate and
//     templates/softdelete.tmpl emits a call to a function that does not exist.
//
// The second is the one worth refusing loudly. The failure it prevents is a
// compile error inside the consumer's own ent package, naming a predicate they
// never wrote, with nothing pointing back at the mixin that asked for it.
func unusableSoftDeleteField(node *gen.Type) string {
	a := softDeleteAnnotation(node)
	if a == nil {
		return ""
	}
	f := softDeleteField(node)
	if f == nil {
		return fmt.Sprintf(
			"%s: carries the %s marker naming field %q, but the entity has no such field. "+
				"Soft delete is opted into by embedding entapi.SoftDeleteMixin, which declares the field it marks; "+
				"attaching %s by hand is not supported",
			node.Name, SoftDeleteAnnotationName, a.Field, SoftDeleteAnnotationName)
	}
	if !f.Optional {
		return fmt.Sprintf(
			"%s.%s: is the soft-delete tombstone field but is not Optional, so ent generates no %s.%sIsNil predicate "+
				"and the generated traverser (templates/softdelete.tmpl) would not compile. "+
				"Use entapi.SoftDeleteMixin, which declares the field Optional and Nillable",
			node.Name, f.Name, node.Package(), f.StructField())
	}
	return ""
}

// supportedIDType and unsupportedIDType used to live here. They refused any
// primary key that was not uuid.UUID, because base_service.tmpl and
// base_handler.tmpl spelled that type into every signature they emitted.
//
// #29 deleted both templates, and with them the only reason the refusal
// existed. dto.tmpl, filter.tmpl and wiring.tmpl all render the identifier
// through $.ID.Type and import its package through wiringImports, so an
// int-keyed or string-keyed entity generates code that compiles — which the
// "intid" fixture now asserts by generating and building, where it previously
// asserted the refusal message.

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
// bare entapi.Edge(). It grants no scope, so nothing about the output
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
			"Declare the two ends separately and give each its own annotation — edge.To(%q, %s.Type).Annotations(entapi.Edge().InResponse()) "+
			"and edge.From(%q, %s.Type).Ref(%q).Annotations(entapi.Edge().InResponse()) — "+
			"or, to expose %q alone on purpose, annotate %q with a bare entapi.Edge(), which grants no scope and says the end was considered",
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
				"add %s to the field's scopes (entapi.DefaultField(), CreateOnlyField() and OutputOnlyField() all carry it), or drop the marker",
			node.Name, f.Name, markerList(a), ScopeQuery, ScopeQuery,
		))
	}

	if a.Searchable && !fieldHasOp(f, gen.Contains) {
		out = append(out, fmt.Sprintf(
			"%s.%s: annotation marks the field Searchable, but ent derives no Contains predicate for type %q "+
				"(entc/gen/func.go fieldOps), so there is no %sContains to put in the free-text disjunction. "+
				"Free-text search is a substring match and only string fields have one — drop AsSearchable(); AsFilterable() offers this type's non-substring operators",
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

// errorMapSymbol is the only EXPORTED name this extension declares once per
// graph, spelled exactly as templates/errors.tmpl spells it.
//
// softdelete.tmpl declares only softDeleteTraverser and softDeleteHook. An ent
// schema type is always exported, so those names cannot collide with one and
// do not belong in the reserved set.
const errorMapSymbol = "ErrorMap"

// reservedNameConflicts reports every entity whose NAME is also a name this
// extension declares in the package it generates into.
//
// The failure it prevents (#62): ent generates `type ErrorMap` for an entity
// called ErrorMap, templates/errors.tmpl generates `var ErrorMap` into
// entapi_errors.go, and Go gives types, variables and functions ONE
// identifier namespace per package — so the consumer's own ent package stops
// compiling with `ErrorMap redeclared in this block`, in two files they did not
// write, with nothing naming the extension as the cause.
//
// It is a graph-level check for two reasons, and both are why it runs outside
// checkGraphConflicts' per-node loop:
//
//   - whether the graph-level error file is emitted at all is a property of the
//     graph (any annotated entity), not of the node being examined;
//   - the colliding entity does NOT have to be annotated. ent generates a type
//     for every entity in the schema, annotated or not, so a bare `type
//     ErrorMap struct{ ent.Schema }` collides just as hard — and the per-node
//     loop skips exactly those.
//
// The derived half is checked against the MAXIMAL set of names an annotated
// entity can produce, never against the set this particular entity's scopes
// happen to trigger. An entity with no create-scoped field emits no
// <Name>CreateRequest today, but adding one scope later would, and a refusal
// that appears only after an unrelated annotation change is worse than one that
// is stable. Refusing a name that would not have collided is the accepted cost.
func reservedNameConflicts(g *gen.Graph) []string {
	var annotated []*gen.Type
	for _, node := range g.Nodes {
		if len(domainFields(node)) > 0 {
			annotated = append(annotated, node)
		}
	}

	var out []string

	// The graph-level name is gated on the condition its file is actually
	// written under (see generatePerTypeFiles): the error classifier is emitted
	// when anything is annotated.
	if len(annotated) > 0 {
		out = append(out, graphSymbolConflicts(g, errorMapSymbol, "var", errorMapFileName,
			annotated, "carries entapi annotations, so the error classifier is generated for this schema")...)
	}

	// The derived half: every pair of (annotated entity, any entity) where the
	// second one's name is a name the first one's generated files declare.
	for _, a := range annotated {
		for _, d := range derivedEntityDecls(a) {
			for _, b := range g.Nodes {
				if b.Name == d.name {
					out = append(out, derivedNameConflict(a, b, d))
				}
			}
		}
	}

	return out
}

// graphSymbolConflicts describes an entity named after a symbol this extension
// declares once per graph.
//
// causes are the entities that make the file be generated at all, and one of
// them is named in the message, because that is the fact the author cannot see:
// entapi_errors.go carries no entity name, and an author staring at a
// redeclared ErrorMap has no way to tell which annotation summoned it. An entity
// other than the colliding one is preferred as the witness — "ErrorMap is
// annotated, therefore ErrorMap is generated" is true but says nothing.
func graphSymbolConflicts(g *gen.Graph, name, kind, file string, causes []*gen.Type, because string) []string {
	witness := causes[0]
	for _, c := range causes {
		if c.Name != name {
			witness = c
			break
		}
	}
	why := witness.Name + " " + because

	var out []string
	for _, node := range g.Nodes {
		if node.Name != name {
			continue
		}
		out = append(out, fmt.Sprintf(
			"%s: the schema declares an entity named %s, so ent generates `type %s` for it; this extension declares `%s %s` in %s (%s), "+
				"and Go gives types, variables and functions one identifier namespace per package, so the generated package fails to compile with `%s redeclared in this block` — in two files the author did not write. "+
				"So rename the entity: %s is a documented part of this package's API (README.md's init() example assigns to ErrorMap), and moving it would break every consumer that already refers to it",
			node.Name, name, name, kind, name, file, why, name, name,
		))
	}
	return out
}

// derivedName is one exported top-level declaration the per-type templates
// derive from an entity's name, together with the file it lands in.
type derivedName struct {
	name string
	file string
}

// derivedEntityDecls returns every EXPORTED top-level declaration the three
// per-type templates emit for an annotated node.
//
// This is the list #62 turns on, and it is the one thing here that rots: a
// template gains a declaration and this list does not, and the collision it was
// written to refuse walks straight through. That is why
// TestDerivedEntityNamesMatchTheTemplates renders all five standalone output
// templates over a probe entity, reads the exported declarations back out with
// go/parser, and compares the two sets in BOTH directions. Add to a template,
// and that test tells you to add here — no reading required.
//
// Unexported declarations (<name>SortOptions, <name>ByID, <name>Get, the
// presence tag slices) are deliberately absent: an ent schema type's name is
// always exported, so an unexported generated name cannot be collided with.
//
// Methods are absent for the same class of reason — a method name lives in its
// receiver's namespace, not the package's, so Predicates, Validate and Apply
// collide with nothing.
//
// The plural forms go through ent's own plural function rather than a rule
// written here. templates/wiring.tmpl calls `plural` from gen.Funcs, so a second
// implementation could only ever disagree with the names actually generated —
// and it would disagree exactly on the irregular nouns nobody tests with.
func derivedEntityDecls(node *gen.Type) []derivedName {
	n := node.Name
	p := entPlural(n)
	dto := perTypeFileName(node, "dto")
	filter := perTypeFileName(node, "filter")
	wiring := perTypeFileName(node, "wiring")

	return []derivedName{
		// templates/dto.tmpl
		{n + "CreateRequest", dto},
		{"Valid" + n + "CreateRequest", dto},
		{n + "PatchRequest", dto},
		{"Valid" + n + "PatchRequest", dto},
		{n + "Summary", dto},
		{"New" + n + "Summary", dto},
		{n + "Response", dto},
		{"New" + n + "Response", dto},
		{n + "QueryWithResponseEdges", dto},
		{n + "ListResponse", dto},
		{"New" + n + "ListResponse", dto},
		// templates/filter.tmpl
		{n + "Filter", filter},
		{n + "SortKeys", filter},
		{n + "Order", filter},
		// templates/wiring.tmpl
		{"Get" + n, wiring},
		{"List" + p, wiring},
		{"Create" + n, wiring},
		{"Patch" + n, wiring},
		{"Delete" + n, wiring},
		{"DeleteBatch" + p, wiring},
	}
}

// derivedEntityNames is derivedEntityDecls projected onto the identifiers
// alone. It is what a caller that only asks "is this name taken?" wants, and it
// is the form the consistency guard compares against.
func derivedEntityNames(node *gen.Type) []string {
	decls := derivedEntityDecls(node)
	names := make([]string, 0, len(decls))
	for _, d := range decls {
		names = append(names, d.name)
	}
	return names
}

// entPlural is ent's own plural function, reached through the same map the
// templates reach it through.
//
// Bound at package init, and panicking there, for the reason mustLoadTemplate
// panics at init: a generator that cannot spell the names it is about to
// generate has nothing correct left to do, and finding that out when the first
// consumer runs `go generate` is strictly worse than finding it out on import.
var entPlural = mustEntPlural()

func mustEntPlural() func(string) string {
	fn, ok := gen.Funcs["plural"].(func(string) string)
	if !ok {
		panic("entapi: entc/gen no longer exposes \"plural\" as func(string) string, " +
			"so the derived names in schema_conflicts.go can no longer be spelled the way templates/wiring.tmpl spells them")
	}
	return fn
}

// derivedNameConflict describes an entity named after a symbol another entity's
// generated files declare — entity FooResponse against entity Foo's generated
// FooResponse.
//
// The pair is named in the subject line because neither entity is wrong on its
// own and the author has to pick which one moves. The message says which name is
// derived and which is free, because that is what makes the choice obvious:
// Foo's derived names follow from Foo's name and cannot be configured.
func derivedNameConflict(a, b *gen.Type, d derivedName) string {
	return fmt.Sprintf(
		"%s / %s: the schema declares an entity named %s, so ent generates `type %s` for it; %s is also one of the names this extension derives from the annotated entity %s and declares in %s, "+
			"so the two share one identifier in the generated package and it fails to compile with `%s redeclared in this block`. "+
			"So rename one of the two entities — %s's derived names follow from its own name and cannot be configured. "+
			"The name is reserved even if %s's current scopes do not happen to emit it: adding a scope later would, and a refusal that appears on an unrelated annotation change is worse than one that is stable",
		b.Name, a.Name, b.Name, b.Name, d.name, a.Name, d.file, d.name, a.Name, a.Name,
	)
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
// Note that entapi.DefaultField() grants ScopeUpdate, so an immutable field
// carrying the default annotation always lands here. That is intended: the fix
// is one annotation, and it has to be written down somewhere.
func immutableUpdateConflict(node *gen.Type, f *gen.Field) string {
	return fmt.Sprintf(
		"%s.%s: field is Immutable() in the ent schema but its DomainField annotation carries scope %q; "+
			"ent generates no Set%s on %sUpdateOne (update builders iterate MutableFields), so no update request can be generated for it. "+
			"Give the field an annotation without ScopeUpdate (entapi.CreateOnlyField() or entapi.OutputOnlyField()), or drop Immutable() from the field",
		node.Name, f.Name, ScopeUpdate, f.StructField(), node.Name,
	)
}
