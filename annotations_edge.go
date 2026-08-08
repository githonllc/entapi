package entapi

import "entgo.io/ent/entc/gen"

// responseEdgeSet selects expanded edges in schema order.
func responseEdgeSet(node *gen.Type) []*gen.Edge {
	if node == nil {
		return nil
	}
	var edges []*gen.Edge
	for _, edge := range node.Edges {
		if a := getEdgeAnnotation(edge); a != nil && a.Expand {
			edges = append(edges, edge)
		}
	}
	return edges
}

// edgeJSONKey returns the explicit key or the edge storage key.
func edgeJSONKey(edge *gen.Edge) string {
	if a := getEdgeAnnotation(edge); a != nil && a.Key != "" {
		return a.Key
	}
	return edge.Name
}
