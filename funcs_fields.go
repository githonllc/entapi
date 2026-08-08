package entapi

import (
	"fmt"

	"entgo.io/ent/entc/gen"
)

// createFields derives the create request from Ent's fields, removing only
// explicit Hidden and ReadOnly deviations.
func createFields(node *gen.Type) []*gen.Field {
	if node == nil {
		return nil
	}
	var fields []*gen.Field
	for _, field := range node.Fields {
		a := getFieldAnnotation(field)
		if a != nil && (a.Hidden || a.ReadOnly) {
			continue
		}
		fields = append(fields, field)
	}
	return fields
}

// patchFields starts from Ent's own mutable-field set, then removes only
// explicit Hidden and ReadOnly deviations.
func patchFields(node *gen.Type) []*gen.Field {
	if node == nil {
		return nil
	}
	var fields []*gen.Field
	for _, field := range node.MutableFields() {
		a := getFieldAnnotation(field)
		if a != nil && (a.Hidden || a.ReadOnly) {
			continue
		}
		fields = append(fields, field)
	}
	return fields
}

// responseFields derives the scalar response from Ent's fields. Sensitive is
// deliberately the first statement in the loop: Ent's secret marker always
// wins before annotation vocabulary is consulted.
func responseFields(node *gen.Type) []*gen.Field {
	if node == nil {
		return nil
	}
	var fields []*gen.Field
	for _, field := range node.Fields {
		if field.Sensitive() {
			continue
		}
		if a := getFieldAnnotation(field); a != nil && a.Hidden {
			continue
		}
		fields = append(fields, field)
	}
	return fields
}

// responseEdges refuses an expanded edge whose target produces no summary.
func responseEdges(node *gen.Type) ([]*gen.Edge, error) {
	edges := responseEdgeSet(node)
	for _, edge := range edges {
		if edge.Type == nil || !isResource(edge.Type) {
			target := "<unknown>"
			if edge.Type != nil {
				target = edge.Type.Name
			}
			return nil, fmt.Errorf(
				"edge %s.%s carries api.Expand(), but target %s is not an api.Resource, so no %sSummary is generated",
				node.Name, edge.Name, target, target)
		}
	}
	return edges, nil
}
