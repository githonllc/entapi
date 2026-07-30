package entdomain

import (
	"encoding/json"

	"entgo.io/ent/entc/gen"
)

// DomainEdge is the edge-level annotation.
//
// Edges need an annotation of their own because the current rule derives an
// edge's exposure from its foreign-key field's scope, which conflates two
// different intents — "put author_id in the response" and "put a nested author
// object in the response" — and makes exposure depend on which table happens to
// hold the column. A to-many edge has no field on this entity at all, so under
// that rule it can never be exposed.
type DomainEdge struct {
	// Scopes controls where the edge appears. Only ScopeResponse is meaningful
	// today: request structs do not carry nested entities.
	Scopes []FieldScope `json:"scopes,omitempty"`

	// JSONKey overrides the JSON key. Empty means the edge name is used.
	// It is not called Name because Name() is the schema.Annotation method.
	JSONKey string `json:"json_key,omitempty"`
}

// Name implements the schema.Annotation interface.
func (DomainEdge) Name() string { return "DomainEdge" }

// Edge starts an edge annotation. Value receivers throughout: every builder
// returns a copy, matching DomainField's contract.
func Edge() DomainEdge { return DomainEdge{} }

// InResponse marks the edge for inclusion in generated response structs.
func (e DomainEdge) InResponse() DomainEdge {
	if !e.hasScope(ScopeResponse) {
		e.Scopes = append(append([]FieldScope{}, e.Scopes...), ScopeResponse)
	}
	return e
}

// As overrides the JSON key for this edge.
func (e DomainEdge) As(key string) DomainEdge {
	e.JSONKey = key
	return e
}

func (e DomainEdge) hasScope(s FieldScope) bool {
	for _, v := range e.Scopes {
		if v == s {
			return true
		}
	}
	return false
}

// getDomainEdgeAnnotation reads the DomainEdge annotation off an edge.
//
// Like getDomainFieldAnnotation, it must handle both representations: the
// annotation arrives as DomainEdge during codegen but as map[string]interface{}
// when the schema was loaded from a serialized description. A JSON round-trip
// normalizes both.
func getDomainEdgeAnnotation(edge *gen.Edge) *DomainEdge {
	if edge == nil || edge.Annotations == nil {
		return nil
	}
	raw, ok := edge.Annotations["DomainEdge"]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case *DomainEdge:
		return v
	case DomainEdge:
		return &v
	default:
		encoded, err := json.Marshal(raw)
		if err != nil {
			return nil
		}
		var out DomainEdge
		if err := json.Unmarshal(encoded, &out); err != nil {
			return nil
		}
		return &out
	}
}

// hasEdgeScope reports whether an edge carries the given scope.
func hasEdgeScope(edge *gen.Edge, scope FieldScope) bool {
	a := getDomainEdgeAnnotation(edge)
	if a == nil {
		return false
	}
	return a.hasScope(scope)
}

// responseEdgesV2 selects edges for the response by their own annotation.
//
// Unlike responseEdges it does not consult edge.Field(), so a to-many edge —
// whose foreign key lives on the other entity — is selectable.
func responseEdgesV2(node *gen.Type) []*gen.Edge {
	var edges []*gen.Edge
	for _, edge := range node.Edges {
		if hasEdgeScope(edge, ScopeResponse) {
			edges = append(edges, edge)
		}
	}
	return edges
}

// edgeJSONKey returns the JSON key to use for an edge.
func edgeJSONKey(edge *gen.Edge) string {
	if a := getDomainEdgeAnnotation(edge); a != nil && a.JSONKey != "" {
		return a.JSONKey
	}
	return edge.Name
}
