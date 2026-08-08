package entapi

import (
	"encoding/json"

	"entgo.io/ent/entc/gen"

	"github.com/githonllc/entapi/api"
)

const (
	resourceAnnotationName = "EntAPIResource"
	fieldAnnotationName    = "EntAPIField"
	edgeAnnotationName     = "EntAPIEdge"
)

// getResourceAnnotation reads the resource annotation from an Ent type.
//
// Ent annotations are pointers while a schema is being described and
// map[string]interface{} values after the serialized-schema load path. The JSON
// round-trip keeps both paths on one reader contract.
func getResourceAnnotation(node *gen.Type) *api.ResourceAnnotation {
	if node == nil {
		return nil
	}
	return decodeResourceAnnotation(node.Annotations[resourceAnnotationName])
}

func decodeResourceAnnotation(raw interface{}) *api.ResourceAnnotation {
	switch v := raw.(type) {
	case *api.ResourceAnnotation:
		return v
	case api.ResourceAnnotation:
		return &v
	case map[string]interface{}:
		var out api.ResourceAnnotation
		if decodeAnnotation(v, &out) {
			return &out
		}
	}
	return nil
}

// getFieldAnnotation reads the deviation words from an Ent field.
func getFieldAnnotation(field *gen.Field) *api.FieldAnnotation {
	if field == nil {
		return nil
	}
	raw := field.Annotations[fieldAnnotationName]
	switch v := raw.(type) {
	case *api.FieldAnnotation:
		return v
	case api.FieldAnnotation:
		return &v
	case map[string]interface{}:
		var out api.FieldAnnotation
		if decodeAnnotation(v, &out) {
			return &out
		}
	}
	return nil
}

// getEdgeAnnotation reads expansion from an Ent edge.
func getEdgeAnnotation(edge *gen.Edge) *api.EdgeAnnotation {
	if edge == nil {
		return nil
	}
	raw := edge.Annotations[edgeAnnotationName]
	switch v := raw.(type) {
	case *api.EdgeAnnotation:
		return v
	case api.EdgeAnnotation:
		return &v
	case map[string]interface{}:
		var out api.EdgeAnnotation
		if decodeAnnotation(v, &out) {
			return &out
		}
	}
	return nil
}

func decodeAnnotation(raw, dst interface{}) bool {
	data, err := json.Marshal(raw)
	if err != nil {
		return false
	}
	return json.Unmarshal(data, dst) == nil
}

func isResource(node *gen.Type) bool { return getResourceAnnotation(node) != nil }

func resourceExcepts(node *gen.Type, op api.Op) bool {
	a := getResourceAnnotation(node)
	if a == nil {
		return false
	}
	for _, excluded := range a.ExceptOps {
		if excluded == op {
			return true
		}
	}
	return false
}

// blockedCreateFields returns required, non-defaulted fields that a deviation
// word removes from the create request.
func blockedCreateFields(node *gen.Type) []*gen.Field {
	if node == nil {
		return nil
	}
	var blocked []*gen.Field
	for _, f := range node.Fields {
		if f.Optional || f.Default {
			continue
		}
		a := getFieldAnnotation(f)
		if a != nil && (a.Hidden || a.ReadOnly) {
			blocked = append(blocked, f)
		}
	}
	return blocked
}

// hasCreateFamily reports whether templates should emit the create request and
// wiring family. The unusable, non-Excepted quadrant is rejected before
// templates run; returning true there makes Except observable to the surface
// reachability test while keeping that invariant explicit.
func hasCreateFamily(node *gen.Type) bool {
	return len(blockedCreateFields(node)) == 0 || !resourceExcepts(node, api.OpCreate)
}
