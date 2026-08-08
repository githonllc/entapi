package entapi

import (
	"reflect"
	"sort"
	"testing"

	"entgo.io/ent/entc/gen"

	"github.com/githonllc/entapi/api"
)

// pendingKnobs is intentionally empty. The new annotation vocabulary has no
// published storage that generation ignores.
var pendingKnobs = map[string]string{}

type knobProbe struct {
	registeredFunc string
	changesOutput  func() bool
}

func TestEveryAnnotationKnobIsConsumedOrDeclaredPending(t *testing.T) {
	probes := map[string]knobProbe{
		"ResourceAnnotation.ExceptOps": {"hasCreateFamily", exceptChangesCreateFamily},
		"FieldAnnotation.Hidden":       {"responseFields", hiddenChangesResponseFields},
		"FieldAnnotation.ReadOnly":     {"createFields", readOnlyChangesCreateFields},
		"FieldAnnotation.Searchable":   {"isSearchable", func() bool { return queryWordChanges(api.Searchable(), isSearchable) }},
		"FieldAnnotation.Filterable":   {"isFilterable", func() bool { return queryWordChanges(api.Filterable(), isFilterable) }},
		"FieldAnnotation.Sortable":     {"isSortable", func() bool { return queryWordChanges(api.Sortable(), isSortable) }},
		"EdgeAnnotation.Expand":        {"responseEdges", expandChangesResponseEdges},
		"EdgeAnnotation.Key":           {"edgeJSONKey", keyChangesEdgeJSONKey},
	}

	knobs := exportedAnnotationKnobs()
	for _, knob := range knobs {
		probe, ok := probes[knob]
		if !ok {
			t.Errorf("exported knob %q has no reachability probe", knob)
			continue
		}
		if _, ok := templateFuncs()[probe.registeredFunc]; !ok {
			t.Errorf("knob %q only reaches %q, which is not a registered template func", knob, probe.registeredFunc)
		}
		if !probe.changesOutput() {
			t.Errorf("knob %q does not change its registered template func output", knob)
		}
		if reason, pending := pendingKnobs[knob]; pending {
			t.Errorf("reachable knob %q remains pending: %s", knob, reason)
		}
	}
	for knob := range probes {
		if !containsString(knobs, knob) {
			t.Errorf("reachability probe names removed knob %q", knob)
		}
	}
	if len(pendingKnobs) != 0 {
		t.Fatalf("pendingKnobs = %v, want empty", pendingKnobs)
	}
}

func exportedAnnotationKnobs() []string {
	types := []reflect.Type{
		reflect.TypeOf(api.ResourceAnnotation{}),
		reflect.TypeOf(api.FieldAnnotation{}),
		reflect.TypeOf(api.EdgeAnnotation{}),
	}
	var out []string
	for _, typ := range types {
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if field.PkgPath == "" {
				out = append(out, typ.Name()+"."+field.Name)
			}
		}
	}
	sort.Strings(out)
	return out
}

func exceptChangesCreateFamily() bool {
	blocked := newStringField("secret", fieldPtr(api.Hidden()))
	node := newTestType("Probe", blocked)
	before := hasCreateFamily(node)
	node.Annotations[resourceAnnotationName] = resourcePtr(api.Resource().Except(api.OpCreate))
	return before != hasCreateFamily(node)
}

func hiddenChangesResponseFields() bool {
	field := newStringField("value", fieldPtr(api.FieldAnnotation{}))
	node := newTestType("Probe", field)
	before := len(responseFields(node))
	field.Annotations[fieldAnnotationName] = fieldPtr(api.Hidden())
	return before != len(responseFields(node))
}

func readOnlyChangesCreateFields() bool {
	field := newStringField("value", fieldPtr(api.FieldAnnotation{}))
	node := newTestType("Probe", field)
	before := len(createFields(node))
	field.Annotations[fieldAnnotationName] = fieldPtr(api.ReadOnly())
	return before != len(createFields(node))
}

func queryWordChanges(word api.FieldAnnotation, selector func(*gen.Field) bool) bool {
	field := newStringField("value", fieldPtr(api.FieldAnnotation{}))
	before := selector(field)
	field.Annotations[fieldAnnotationName] = fieldPtr(word)
	return before != selector(field)
}

func expandChangesResponseEdges() bool {
	target := newTestType("Target", newStringField("value", nil))
	edge := &gen.Edge{Name: "target", Type: target}
	node := newTestType("Probe", newStringField("value", nil))
	node.Edges = []*gen.Edge{edge}
	before, _ := responseEdges(node)
	edge.Annotations = gen.Annotations{edgeAnnotationName: edgePtr(api.Expand())}
	after, _ := responseEdges(node)
	return len(before) != len(after)
}

func keyChangesEdgeJSONKey() bool {
	edge := &gen.Edge{Name: "target", Annotations: gen.Annotations{edgeAnnotationName: edgePtr(api.EdgeAnnotation{})}}
	before := edgeJSONKey(edge)
	edge.Annotations[edgeAnnotationName] = edgePtr(api.EdgeAnnotation{}.JSONKey("owner"))
	return before != edgeJSONKey(edge)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
