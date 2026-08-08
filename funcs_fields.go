package entapi

import (
	"fmt"

	"entgo.io/ent/entc/gen"
)

// domainFields returns all fields with DomainField annotation
func domainFields(node *gen.Type) []*gen.Field {
	var fields []*gen.Field
	for _, field := range node.Fields {
		if annotation := getDomainFieldAnnotation(field); annotation != nil {
			fields = append(fields, field)
		}
	}
	return fields
}

// createFields returns fields that can be used in create requests
func createFields(node *gen.Type) []*gen.Field {
	var fields []*gen.Field
	for _, field := range node.Fields {
		if annotation := getDomainFieldAnnotation(field); annotation != nil {
			if hasDomainScope(field, ScopeCreate) {
				fields = append(fields, field)
			}
		}
	}
	return fields
}

// patchFields returns the fields a patch request carries, in schema order.
//
// The set is the intersection of two authorities, and ent's is the binding one:
// a field must carry ScopeUpdate, AND it must be one ent's update builders can
// actually set. That second half is taken from gen.Type.MutableFields rather
// than re-derived here, because it is the very list ent's own setter template
// iterates — immutable fields and immutable-edge foreign keys are absent from
// it, so a field that survives this filter provably has a Set<Field> method.
//
// checkGraphConflicts (schema_conflicts.go) refuses an Immutable() field
// carrying ScopeUpdate before generation starts, so today the intersection
// never actually drops anything. That is the point: the refusal is what a
// schema author sees, and this filter is what makes the emitted code correct
// even if a future ent adds another way for a field to be unsettable.
func patchFields(node *gen.Type) []*gen.Field {
	var fields []*gen.Field
	for _, field := range node.MutableFields() {
		if annotation := getDomainFieldAnnotation(field); annotation != nil {
			if hasDomainScope(field, ScopeUpdate) {
				fields = append(fields, field)
			}
		}
	}
	return fields
}

// responseFields returns fields that can be used in responses.
//
// Ent's Sensitive fact narrows the annotation's response scope: a field the
// schema marks Sensitive never reaches either response tier. See doc.go's
// migration note for the old annotation knob that existed but was never read.
func responseFields(node *gen.Type) []*gen.Field {
	var fields []*gen.Field
	for _, field := range node.Fields {
		if field.Sensitive() {
			continue
		}
		if annotation := getDomainFieldAnnotation(field); annotation != nil {
			if hasDomainScope(field, ScopeResponse) {
				fields = append(fields, field)
			}
		}
	}
	return fields
}

// responseEdges returns the edges a response type declares, in schema order.
//
// Selection is by the edge's own DomainEdge annotation (responseEdgeSet), never
// by where the foreign key happens to sit. The rule this replaced required
// edge.Field() != nil, which holds only when the column is on this entity, so a
// to-many edge was permanently unreachable and "expose author_id" and "expose
// the nested author" were forced to be one switch.
//
// It returns an error instead of dropping an edge whose target carries no
// DomainField annotation at all. Such a target is skipped wholesale by the
// generator (extension.go), so no <Target>Summary would exist for the response
// to name: dropping the edge would silently narrow the response the schema
// asked for, and emitting the reference would fail at the consumer's compiler
// with an undefined symbol rather than here with a schema-level explanation.
func responseEdges(node *gen.Type) ([]*gen.Edge, error) {
	edges := responseEdgeSet(node)
	for _, edge := range edges {
		if len(domainFields(edge.Type)) == 0 {
			return nil, fmt.Errorf(
				"edge %s.%s is annotated for the response, but %s has no entapi field annotation, "+
					"so no %sSummary is generated: annotate a field on %s, or drop the edge annotation",
				node.Name, edge.Name, edge.Type.Name, edge.Type.Name, edge.Type.Name)
		}
	}
	return edges, nil
}
