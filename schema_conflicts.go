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

// nodeConflicts returns one human-readable message per contradicting field of
// one entity. Each message names the entity, the field, and both conflicting
// facts, because the schema author has to know which of the two to change.
func nodeConflicts(node *gen.Type) []string {
	var out []string
	for _, f := range node.Fields {
		if getDomainFieldAnnotation(f) == nil {
			continue
		}
		if f.Immutable && hasDomainScope(f, ScopeUpdate) {
			out = append(out, immutableUpdateConflict(node, f))
		}
	}
	return out
}

// immutableUpdateConflict describes the one contradiction this package can
// currently detect: a field ent marks Immutable that carries ScopeUpdate.
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
