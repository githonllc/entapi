package entdomain

import (
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

// updateFields returns fields that can be used in update requests
func updateFields(node *gen.Type) []*gen.Field {
	var fields []*gen.Field
	for _, field := range node.Fields {
		if annotation := getDomainFieldAnnotation(field); annotation != nil {
			if hasDomainScope(field, ScopeUpdate) {
				fields = append(fields, field)
			}
		}
	}
	return fields
}

// responseFields returns fields that can be used in responses
func responseFields(node *gen.Type) []*gen.Field {
	var fields []*gen.Field
	for _, field := range node.Fields {
		if annotation := getDomainFieldAnnotation(field); annotation != nil {
			if hasDomainScope(field, ScopeResponse) {
				fields = append(fields, field)
			}
		}
	}
	return fields
}

// responseEdges returns edges suitable for inclusion in HTTP responses.
// An edge qualifies when: (1) it has a FK field on this entity,
// (2) that FK field has ScopeResponse, and (3) the target type is a domain entity.
func responseEdges(node *gen.Type) []*gen.Edge {
	var edges []*gen.Edge
	for _, edge := range node.Edges {
		if edgeQualifiesForResponse(edge.Field(), edge.Type) {
			edges = append(edges, edge)
		}
	}
	return edges
}

// edgeQualifiesForResponse checks if an edge with the given FK field and target
// type qualifies for inclusion in response structs. Separated from responseEdges
// for testability, since edge.Field() depends on unexported ent internals.
func edgeQualifiesForResponse(fkField *gen.Field, targetType *gen.Type) bool {
	if fkField == nil {
		return false
	}
	if !hasDomainScope(fkField, ScopeResponse) {
		return false
	}
	return len(domainFields(targetType)) > 0
}
