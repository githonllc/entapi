package entdomain

import (
	"fmt"
	"strings"

	"entgo.io/ent/entc/gen"
)

// checkGraphConflicts rejects a graph whose entdomain annotations contradict
// the ent schema they are attached to.
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
// sees every conflict at once rather than one per `go generate` cycle.
func checkGraphConflicts(g *gen.Graph) error {
	var conflicts []string
	for _, node := range g.Nodes {
		conflicts = append(conflicts, nodeConflicts(node)...)
	}
	if len(conflicts) == 0 {
		return nil
	}
	return fmt.Errorf("entdomain: %d field annotation(s) contradict the ent schema:\n  - %s",
		len(conflicts), strings.Join(conflicts, "\n  - "))
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
