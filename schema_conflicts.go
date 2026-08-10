package entapi

import (
	"fmt"
	"strings"

	"entgo.io/ent/entc/gen"

	"github.com/githonllc/entapi/api"
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
// HTTP checks run only for api.Resources; soft-delete and graph-symbol checks
// have their own graph-level gates. This function used to take the extension config, to
// decide whether the identifier-type refusal applied: base_service.tmpl and
// base_handler.tmpl wrote "uuid.UUID" into every signature, so an int-keyed
// entity had no correct output. Those templates are gone (#29) and the
// remaining ones render the id through $.ID.Type, so there is no identifier
// this package refuses and no configuration left to consult.
func checkGraphConflicts(g *gen.Graph) error {
	var conflicts []string
	for _, node := range g.Nodes {
		// Checked before the Resource gate below: soft delete is declared
		// on the ent schema, not on the HTTP surface, so an entity with no
		// annotated field at all can still carry the mixin and still be
		// generated for.
		if msg := unusableSoftDeleteField(node); msg != "" {
			conflicts = append(conflicts, msg)
		}
		if !isResource(node) {
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

// nodeConflicts returns every refusal-matrix row for one Resource.
func nodeConflicts(node *gen.Type) []string {
	var out []string
	if node.ID != nil && getFieldAnnotation(node.ID) != nil {
		out = append(out, fmt.Sprintf(
			"%s.%s: the primary key carries api.%s(), but ID has a fixed HTTP shape and typ.ID is not part of typ.Fields; remove every field deviation word from the id",
			node.Name, node.ID.Name, markerList(getFieldAnnotation(node.ID))))
	}
	if node.ID != nil && hasRawAnnotation(node.ID.Annotations, edgeAnnotationName) {
		out = append(out, misplacedExpandConflict(node, node.ID))
	}
	if kind, _, _ := fieldParseConversion(node.ID); node.ID != nil && kind == "" {
		out = append(out, fmt.Sprintf(
			"%s.%s: the primary key is naturally Filterable, but type %q cannot parse a query wire value; use a basic scalar, enum, time.Time, or a type implementing encoding.TextUnmarshaler",
			node.Name, node.ID.Name, node.ID.Type.String(),
		))
	}

	for _, f := range node.Fields {
		a := getFieldAnnotation(f)
		if a != nil {
			out = append(out, fieldWordConflicts(node, f, a)...)
			out = append(out, queryConflicts(node, f, a)...)
		}
		if hasRawAnnotation(f.Annotations, edgeAnnotationName) {
			out = append(out, misplacedExpandConflict(node, f))
		}
	}

	for _, f := range blockedCreateFields(node) {
		if resourceExcepts(node, api.OpCreate) {
			continue
		}
		a := getFieldAnnotation(f)
		word := "ReadOnly"
		if a.Hidden {
			word = "Hidden"
		}
		out = append(out, fmt.Sprintf(
			"%s.%s: field is required by Ent and has no Default, but api.%s() keeps it out of the create request, so every create would fail. Repair it by adding api.Resource().Except(api.OpCreate), by making the field Optional or giving it a Default, or by removing api.%s()",
			node.Name, f.Name, word, word))
	}

	if len(patchFields(node)) == 0 && !resourceExcepts(node, api.OpPatch) {
		out = append(out, fmt.Sprintf(
			"%s: the derived PATCH field set is empty, so the PATCH endpoint would be useless; add api.Resource().Except(api.OpPatch), or leave at least one mutable field outside api.Hidden()/api.ReadOnly()",
			node.Name))
	}

	for _, edge := range node.Edges {
		if hasRawAnnotation(edge.Annotations, fieldAnnotationName) {
			out = append(out, fmt.Sprintf(
				"%s.%s: an api field deviation word is attached to an edge, where it has no reader; use api.Expand() for an edge or move the field word to a field",
				node.Name, edge.Name))
		}
		if a := getEdgeAnnotation(edge); a != nil && a.Expand && (edge.Type == nil || !isResource(edge.Type)) {
			target := "<unknown>"
			if edge.Type != nil {
				target = edge.Type.Name
			}
			out = append(out, fmt.Sprintf(
				"%s.%s: api.Expand() targets %s, but %s is not an api.Resource and therefore has no generated Summary; add api.Resource() to the target or remove api.Expand()",
				node.Name, edge.Name, target, target))
		}
	}

	out = append(out, requiredEdgeWithoutFieldConflicts(node)...)
	out = append(out, asymmetricSelfEdgeConflicts(node)...)
	out = append(out, patchMethodCollisions(node)...)
	return out
}

// patchMethodCollisions reports every patch-visible field whose Go name is
// already a method templates/dto.tmpl declares on the same receiver.
//
// It is the method-level sibling of reservedNameConflicts, and neither check
// subsumes the other. That one compares ENTITY names against the package's
// identifier namespace; a method name lives in its receiver's namespace
// instead, which is exactly why derivedEntityDecls leaves Validate and Apply
// out. What #113 changed is that a method name is now DERIVED FROM A FIELD's Go
// name — the value reader is spelled <Field>() — so two field names can now
// collide with a method the same templates emit.
//
// Two ways that happens, and they are not the same age:
//
//   - a field called "apply". Its reader is `Apply() (T, bool)` on
//     Valid<Entity>PatchRequest, where `Apply(b *<Entity>UpdateOne)` already
//     lives. This one is NEW, introduced by the readers.
//   - a pair "x" and "has_x". Their Go names are X and HasX, and HasX is also
//     the presence method generated for X. This one is OLDER than the readers:
//     #98 put Has<Field>() on the RAW request, where has_x is a struct field of
//     the same name, so `field and method with the same name HasX` was already
//     a compile error before the readers existed. The readers add a second
//     occurrence, on the wrapper. The message names both, because which one the
//     compiler reports first is not something the author can influence.
//
// Gated on patch VISIBILITY and nothing else. A field ent marks Immutable()
// derives out of patchFields, so no reader and no presence method exist for it
// and there would be nothing to collide with. Except(api.OpPatch) is
// deliberately NOT a gate: templates/dto.tmpl renders the patch request, its
// wrapper, Apply and the readers unconditionally — Except removes routes and
// wiring, never a request DTO — so the colliding methods reach the consumer's
// package either way. internal/fixtures/wiring/wiringent/patchless_dto.go is
// the standing proof: its entity Excepts api.OpPatch and still carries
// ValidPatchlessPatchRequest.Apply.
func patchMethodCollisions(node *gen.Type) []string {
	fields := patchFields(node)

	var out []string
	for _, f := range fields {
		if f.StructField() == "Apply" {
			out = append(out, patchApplyCollision(node, f))
		}
		for _, other := range fields {
			if f.StructField() == "Has"+other.StructField() {
				out = append(out, patchPresenceCollision(node, f, other))
			}
		}
	}
	return out
}

// patchApplyCollision describes a patch field whose reader is Apply.
//
// The repair names StorageKey because the wire format does not have to move
// with the Go name: templates/dto.tmpl spells every JSON tag from
// f.StorageKey(), so `field.String("apply_mode").StorageKey("apply")` keeps the
// key the callers already send while giving the reader a name of its own.
func patchApplyCollision(node *gen.Type, f *gen.Field) string {
	return fmt.Sprintf(
		"%s.%s: the field's Go name is Apply, so the patch value reader generated for it is `func (v *Valid%sPatchRequest) Apply() (%s, bool)` — "+
			"but Valid%sPatchRequest already declares `Apply(b *%sUpdateOne)`, and a receiver cannot declare one method name twice, "+
			"so the generated package fails to compile with `method Apply already declared` in a file the author did not write. "+
			"So rename the ent field; the wire key can stay where it is with .StorageKey(%q), which is what the JSON tag is spelled from",
		node.Name, f.Name, node.Name, f.Type, node.Name, node.Name, f.StorageKey(),
	)
}

// patchPresenceCollision describes the pair whose Go names are X and HasX.
//
// The pair is named in the subject line because neither field is wrong on its
// own and the author has to pick which one moves — the same reason
// derivedNameConflict names two entities.
func patchPresenceCollision(node *gen.Type, f, other *gen.Field) string {
	return fmt.Sprintf(
		"%s.%s / %s.%s: the field's Go name is %s, which is also the presence method generated for %s — "+
			"`Has%s()` is declared on %sPatchRequest, where %s is a struct field of that very name, and again on Valid%sPatchRequest, where the value reader generated for %s is spelled %s() as well. "+
			"A struct field and a method cannot share a name and a receiver cannot declare one method name twice, "+
			"so the generated package fails to compile with `field and method with the same name %s` in files the author did not write. "+
			"So rename one of the two ent fields; the wire key can stay where it is with .StorageKey(%q), which is what the JSON tag is spelled from",
		node.Name, f.Name, node.Name, other.Name, f.StructField(), other.StructField(),
		other.StructField(), node.Name, f.StructField(), node.Name, f.Name, f.StructField(),
		f.StructField(), f.StorageKey(),
	)
}

// requiredEdgeWithoutFieldConflicts reports every edge Ent marks Required() on
// a Resource whose create family is reachable, but which declares no
// edge.Field(...).
//
// It is the edge-side twin of blockedCreateFields, and it exists because that
// one cannot see it: blockedCreateFields walks node.Fields, and so does
// createFields, so an edge is invisible to both. The create family is
// nonetheless dead on arrival — ent's generated create.tmpl iterates
// EdgesWithID filtered by `not $e.Optional` and its check() returns
// `missing required edge "X.y"` before any SQL, while nothing puts a setter
// for that edge in {Entity}CreateRequest. Every POST would fail at run time,
// against a request type that offers no way to make it succeed.
//
// Gated on Except(api.OpCreate) exactly like blockedCreateFields: with no
// public create there is no request to be unable to satisfy, which is how the
// softdelete fixture's Draft and the consumer Session it was transplanted from
// keep generating.
func requiredEdgeWithoutFieldConflicts(node *gen.Type) []string {
	if resourceExcepts(node, api.OpCreate) {
		return nil
	}
	var out []string
	for _, edge := range node.Edges {
		if edge.Optional || edge.Field() != nil {
			continue
		}
		out = append(out, requiredEdgeWithoutFieldConflict(node, edge))
	}
	return out
}

// requiredEdgeWithoutFieldConflict describes one such edge.
//
// The remedy list branches on Edge.OwnFK(), not on Edge.Unique, because OwnFK
// is ent's OWN test for whether edge.Field(...) may be declared at all:
// entc/gen/type.go rejects `edge %q has a field %q but it is not holding a
// foreign key` for any other end. Unique is necessary but not sufficient: an
// O2O owns the key only when it is the inverse end or a self-referential Bidi
// one, so the assoc end of a two-type edge.To(...).Unique() / edge.From(...)
// pair is unique and still cannot name the key. Branching on Unique would hand
// that author a repair ent refuses, which is a worse failure than the one being
// reported.
func requiredEdgeWithoutFieldConflict(node *gen.Type, edge *gen.Edge) string {
	repair := "Repair it by adding api.Resource().Except(api.OpCreate), or by making the edge Optional"
	if edge.OwnFK() {
		repair = "Repair it by adding edge.Field(...) so the foreign key becomes an ordinary Ent field the create request can carry, " +
			"by adding api.Resource().Except(api.OpCreate), or by making the edge Optional"
	}
	return fmt.Sprintf(
		"%s.%s: Ent marks the edge Required(), so every create must set it, but the edge declares no edge.Field(...), "+
			"so the foreign key is not an Ent field and no setter for it reaches %sCreateRequest; "+
			"every create would fail ent's own `missing required edge` check before any SQL. %s",
		node.Name, edge.Name, node.Name, repair)
}

func hasRawAnnotation(annotations gen.Annotations, name string) bool {
	if annotations == nil {
		return false
	}
	_, ok := annotations[name]
	return ok
}

func misplacedExpandConflict(node *gen.Type, f *gen.Field) string {
	return fmt.Sprintf(
		"%s.%s: api.Expand() is attached to a field, where it has no reader; use one of the five api field words on fields or move api.Expand() to an edge",
		node.Name, f.Name)
}

func fieldWordConflicts(node *gen.Type, f *gen.Field, a *api.FieldAnnotation) []string {
	var out []string
	if a.Hidden {
		others := markerList(&api.FieldAnnotation{
			ReadOnly: a.ReadOnly, Searchable: a.Searchable,
			Filterable: a.Filterable, Sortable: a.Sortable,
		})
		if others != "" {
			out = append(out, fmt.Sprintf(
				"%s.%s: api.Hidden() conflicts with api.%s(); Hidden removes the field from every HTTP surface, so remove Hidden or every other deviation word",
				node.Name, f.Name, others))
		}
	}
	if f.Sensitive() {
		if dimensions := dimensionList(a); dimensions != "" {
			out = append(out, fmt.Sprintf(
				"%s.%s: Ent marks the field Sensitive(), but api.%s() would expose a query oracle over the secret; remove the query word or Sensitive()",
				node.Name, f.Name, dimensions))
		}
		if a.ReadOnly {
			out = append(out, fmt.Sprintf(
				"%s.%s: Ent marks the field Sensitive() while api.ReadOnly() asks to return it; use api.Hidden() for a service-managed secret",
				node.Name, f.Name))
		}
	}
	if dimensions := dimensionList(a); dimensions != "" {
		if resourceExcepts(node, api.OpList) {
			out = append(out, fmt.Sprintf(
				"%s.%s: api.%s() has no reachable query surface because the resource Excepts api.OpList; remove the query word or keep OpList",
				node.Name, f.Name, dimensions))
		}
		if strings.HasPrefix(f.StorageKey(), "_") {
			out = append(out, fmt.Sprintf(
				"%s.%s: query word api.%s() exposes storage key %q, but '_' is reserved for framework query parameters; change the field StorageKey or remove the query word",
				node.Name, f.Name, dimensions, f.StorageKey()))
		}
	}
	return out
}

// asymmetricSelfEdgeConflicts reports every self-referential edge pair whose
// two ends disagree about whether they carry EntAPIEdge at all.
//
// The pair is found from the inverse end: gen resolves Edge.Ref to the assoc
// edge it names, and a self-referential pair is one whose target is the entity
// that owns it, so both ends sit in the same Edges() slice and the author has
// them both in view.
//
// Only "annotated" versus "not annotated at all" counts. Two ends carrying
// different annotations are two decisions the author wrote down, and this
// package has no standing to overrule them; two bare ends are one decision, not
// to expose the relationship. A deliberate one-way choice marks the excluded
// end with api.EdgeAnnotation{}, so both ends are annotated. It is the mixed
// case that has no reading as explicit intent, because the chained declaration
// produces exactly it by accident.
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
		assocAnnotated := getEdgeAnnotation(assoc) != nil
		inverseAnnotated := getEdgeAnnotation(inverse) != nil
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
// The check compares annotation presence, not EdgeAnnotation.Expand. As #79
// established, a deliberate one-way pair is expressible by marking the
// excluded end api.EdgeAnnotation{}: both ends are then annotated, and the
// refusal catches only the truly bare end, which is exactly the chained-builder
// accident. This uses the same zero-value-annotation survival that
// api.Resource() has always relied on, so it is not a fluke of one code path.
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

	return fmt.Sprintf(
		"%s.%s / %s.%s: the two ends of this self-referential edge pair disagree — %s.%s carries api.Expand() while %s.%s carries no EntAPIEdge annotation at all, "+
			"so %q appears in no response type and in no eager-load plan, and nothing else in generation says so; %s. "+
			"Declare the two ends separately and give both api.Expand(); or remove api.Expand() from both; or, if the asymmetry is deliberate, mark the excluded end api.EdgeAnnotation{} — a present-but-not-expanded annotation that says you considered this end and left it out",
		node.Name, assoc.Name, node.Name, inverse.Name,
		node.Name, annotated.Name, node.Name, bare.Name,
		bare.Name, cause,
	)
}

// queryConflicts reports the contradictions between a field's query markers and
// what ent generates for that field.
//
// All three have the same shape: the
// annotation asks for a call to something ent never wrote. Emitting it anyway
// produces an undefined symbol inside the consumer's own ent package, naming
// ent's API, with nothing pointing back at the annotation that asked for it.
// Silently dropping the marker instead would be worse — the parameter would
// vanish from the query API without a word, and the caller would find out by
// getting unfiltered results.
func queryConflicts(node *gen.Type, f *gen.Field, a *api.FieldAnnotation) []string {
	var out []string

	if a.Searchable && !fieldHasOp(f, gen.Contains) {
		out = append(out, fmt.Sprintf(
			"%s.%s: annotation marks the field Searchable, but ent derives no Contains predicate for type %q "+
				"(entc/gen/func.go fieldOps), so there is no %sContains to put in the free-text disjunction. "+
				"Free-text search is a substring match and only string fields have one — remove api.Searchable(); api.Filterable() offers this type's non-substring operators",
			node.Name, f.Name, f.Type.String(), f.StructField(),
		))
	}

	if a.Filterable && len(f.Ops()) == 0 {
		out = append(out, fmt.Sprintf(
			"%s.%s: annotation marks the field Filterable, but ent derives no predicates at all for type %q "+
				"(entc/gen/func.go fieldOps), so the filter group would be empty and the parameter would silently do nothing. Remove api.Filterable()",
			node.Name, f.Name, f.Type.String(),
		))
	}

	if kind, _, _ := fieldParseConversion(f); a.Filterable && len(f.Ops()) > 0 && kind == "" {
		out = append(out, fmt.Sprintf(
			"%s.%s: annotation marks the field Filterable and ent derives predicates, but type %q cannot parse a query wire value; use a basic scalar, enum, time.Time, or a type implementing encoding.TextUnmarshaler, or remove api.Filterable()",
			node.Name, f.Name, f.Type.String(),
		))
	}

	if a.Sortable && (f.Type == nil || !f.Type.Comparable()) {
		out = append(out, fmt.Sprintf(
			"%s.%s: annotation marks the field Sortable, but type %q is not comparable, so ent's order builders skip it "+
				"(entc/gen/template/dialect/sql/meta.tmpl) and there is no %s to put in the sort allow-list. Remove api.Sortable()",
			node.Name, f.Name, f.Type.String(), f.OrderName(),
		))
	}

	return out
}

// markerList names the query markers a field carries, so the scope message says
// which annotation the author has to reconcile.
func markerList(a *api.FieldAnnotation) string {
	var set []string
	if a.Hidden {
		set = append(set, "Hidden")
	}
	if a.ReadOnly {
		set = append(set, "ReadOnly")
	}
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

func dimensionList(a *api.FieldAnnotation) string {
	copy := *a
	copy.Hidden = false
	copy.ReadOnly = false
	return markerList(&copy)
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

const (
	apiSymbol        = "API"
	apiHandlerSymbol = "APIHandler"
	apiOptionSymbol  = "APIOption"
)

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
//     graph (any Resource), not of the node being examined;
//   - the colliding entity does NOT have to be annotated. ent generates a type
//     for every entity in the schema, annotated or not, so a bare `type
//     ErrorMap struct{ ent.Schema }` collides just as hard — and the per-node
//     loop skips exactly those.
//
// The derived half is checked against the MAXIMAL set of names a Resource can
// produce, not only the families this particular Resource emits today. A
// refusal that appears only after an unrelated deviation change is worse than
// a stable reserved set. Refusing a name that would not collide today is the
// accepted cost.
func reservedNameConflicts(g *gen.Graph) []string {
	var annotated []*gen.Type
	for _, node := range g.Nodes {
		if isResource(node) {
			annotated = append(annotated, node)
		}
	}

	var out []string

	// The graph-level name is gated on the condition its file is actually
	// written under (see generatePerTypeFiles): the error classifier is emitted
	// when anything is annotated.
	if len(annotated) > 0 {
		out = append(out, graphSymbolConflicts(g, errorMapSymbol, "var", errorMapFileName,
			annotated, "carries api.Resource(), so the error classifier is generated for this schema")...)
		out = append(out, graphSymbolConflicts(g, apiSymbol, "func", httpFileName,
			annotated, "carries api.Resource(), so the HTTP route tree is generated for this schema")...)
		out = append(out, graphSymbolConflicts(g, apiHandlerSymbol, "type", httpFileName,
			annotated, "carries api.Resource(), so the HTTP route tree is generated for this schema")...)
		out = append(out, graphSymbolConflicts(g, apiOptionSymbol, "type", httpFileName,
			annotated, "carries api.Resource(), so the HTTP route tree is generated for this schema")...)
	}

	// The derived half: every pair of (Resource, any entity) where the
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
				"So rename the entity: %s is a documented generated API symbol, and moving it would break every consumer that already refers to it",
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

// derivedEntityDecls returns every EXPORTED top-level declaration the four
// per-type templates can emit for a Resource node.
//
// This is the list #62 turns on, and it is the one thing here that rots: a
// template gains a declaration and this list does not, and the collision it was
// written to refuse walks straight through. That is why
// TestDerivedEntityNamesMatchTheTemplates renders all seven standalone output
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
// collide with no ENTITY name. They can collide with each other, which is a
// different check: patchMethodCollisions covers the two method names #113
// derives from a FIELD's Go name.
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
	handler := perTypeFileName(node, "handler")

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
		{n + "CreateRequestTags", dto},
		{n + "PatchRequestTags", dto},
		// templates/filter.tmpl
		{n + "Filter", filter},
		{"Parse" + n + "Query", filter},
		{n + "SortKeys", filter},
		{n + "Order", filter},
		// templates/wiring.tmpl
		{"Get" + n, wiring},
		{"List" + p, wiring},
		{"Create" + n, wiring},
		{"Patch" + n, wiring},
		{"Delete" + n, wiring},
		{"DeleteBatch" + p, wiring},
		// templates/handler.tmpl — maximum breadth, independent of Except.
		{"List" + p + "Fn", handler},
		{"Create" + n + "Fn", handler},
		{"Get" + n + "Fn", handler},
		{"Patch" + n + "Fn", handler},
		{"Delete" + n + "Fn", handler},
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
			"The name is reserved even if %s's current operation families do not emit it: a refusal that appears on an unrelated deviation change is worse than one that is stable",
		b.Name, a.Name, b.Name, b.Name, d.name, a.Name, d.file, d.name, a.Name, a.Name,
	)
}
