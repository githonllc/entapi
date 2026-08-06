package entdomain

import (
	"encoding/json"
	"sort"

	"entgo.io/ent/entc/gen"
)

// This file is the codegen side of the soft-delete mixin (#18). The mixin
// itself is in softdelete.go and needs no generation; what does need it is the
// traverser, because ent's query builders cannot be reached from a library
// type:
//
//   - ent.Query is literally `Query any` (ent.go:390), so a traverser written
//     in this package receives an empty interface.
//   - *<T>Query has no WhereP. Across every ent template WhereP appears only on
//     mutations and on the optional intercept wrapper, never on a query
//     builder. A query builder exposes Where(...predicate.<T>), and predicate.<T>
//     is a per-entity named type this package cannot name.
//
// Generating into the consumer's ent package dissolves both: inside that
// package *DocQuery and doc.DeletedAtIsNil() are ordinary local names. What is
// generated is one type switch over the soft-deletable entities, not a method
// injected onto ent's own types — injecting WhereP onto *<T>Query was
// considered and rejected on #18, because ent adding that method later would
// give every consumer a duplicate-method compile error on upgrade.

// isSoftDeletable reports whether a node carries the mixin's marker.
//
// The question is answered by the annotation alone. It is deliberately not
// answered by looking for a field named deleted_at: that convention could not
// tell an entity that opted in from one that merely owns a column with that
// name, and it is the shape internal/fixtures/softdelete/Ledger exists to pin.
func isSoftDeletable(node *gen.Type) bool {
	return softDeleteAnnotation(node) != nil
}

// softDeleteAnnotation returns the DomainSoftDelete annotation on node, or nil.
//
// Like getDomainFieldAnnotation it normalises through a JSON round-trip: the
// annotation arrives as a DomainSoftDelete during codegen but as a
// map[string]interface{} when the schema was loaded from its serialized form,
// and reading the concrete type directly works only on the first path.
func softDeleteAnnotation(node *gen.Type) *DomainSoftDelete {
	if node == nil || node.Annotations == nil {
		return nil
	}
	raw, ok := node.Annotations[SoftDeleteAnnotationName]
	if !ok || raw == nil {
		return nil
	}
	if a, ok := raw.(DomainSoftDelete); ok {
		return &a
	}
	if a, ok := raw.(*DomainSoftDelete); ok {
		return a
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var a DomainSoftDelete
	if err := json.Unmarshal(data, &a); err != nil {
		return nil
	}
	return &a
}

// softDeleteTypes returns the graph's soft-deletable entities, in the graph's
// own order.
//
// Unlike every per-type generator in this package it does NOT skip a node with
// no domain fields. Soft delete is a property of the ent schema, not of the
// HTTP surface: an entity with no annotated field at all still has rows, and a
// query against it still has to exclude the deleted ones.
func softDeleteTypes(g *gen.Graph) []*gen.Type {
	if g == nil {
		return nil
	}
	var out []*gen.Type
	for _, node := range g.Nodes {
		if isSoftDeletable(node) {
			out = append(out, node)
		}
	}
	return out
}

// softDeleteField returns the tombstone *gen.Field of a soft-deletable entity.
//
// The name comes from the annotation rather than from a constant in this
// package, so the generator never hardcodes the column: the schema declares it,
// and checkSoftDeleteConflicts has already refused any graph where it does not
// resolve. Returning nil here would therefore mean the refusal was skipped,
// which the template cannot repair — so the template is written to render only
// what this returns and generation would fail on invalid Go rather than emit
// something subtly wrong.
func softDeleteField(node *gen.Type) *gen.Field {
	a := softDeleteAnnotation(node)
	if a == nil {
		return nil
	}
	for _, f := range node.Fields {
		if f.Name == a.Field {
			return f
		}
	}
	return nil
}

// softDeleteImports returns the import specs the traverser file needs beyond
// the ones it names itself (context, time, entdomain): one entity subpackage
// per soft-deletable type, which is where ent puts the DeletedAtIsNil
// predicate.
func softDeleteImports(g *gen.Graph) []string {
	if g == nil || g.Config == nil {
		return nil
	}
	specs := make(map[string]bool)
	for _, node := range softDeleteTypes(g) {
		specs[quoteImport("", g.Config.Package+"/"+node.Package())] = true
	}
	out := make([]string, 0, len(specs))
	for spec := range specs {
		out = append(out, spec)
	}
	sort.Strings(out)
	return out
}
