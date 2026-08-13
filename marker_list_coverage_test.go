package entapi

import (
	"reflect"
	"strings"
	"testing"

	"github.com/githonllc/entapi/api"
)

func TestMarkerListCoversEveryFieldAnnotationBool(t *testing.T) {
	typ := reflect.TypeOf(api.FieldAnnotation{})
	prefix := typ.Name() + "."
	ran := 0

	for _, knob := range exportedAnnotationKnobs() {
		if !strings.HasPrefix(knob, prefix) {
			continue
		}
		name := strings.TrimPrefix(knob, prefix)
		field, ok := typ.FieldByName(name)
		if !ok {
			t.Errorf("exportedAnnotationKnobs returned %q, want a field on %s", knob, typ.Name())
			continue
		}
		if field.Type.Kind() != reflect.Bool {
			continue
		}
		ran++

		t.Run(name, func(t *testing.T) {
			annotation := api.FieldAnnotation{}
			reflect.ValueOf(&annotation).Elem().FieldByIndex(field.Index).SetBool(true)

			if got := markerList(&annotation); got != name {
				t.Errorf("markerList(%+v) = %q, want %q", annotation, got, name)
			}

			wantDimension := name
			if name == "Hidden" || name == "ReadOnly" {
				wantDimension = ""
			}
			if got := dimensionList(&annotation); got != wantDimension {
				t.Errorf("dimensionList(%+v) = %q, want %q", annotation, got, wantDimension)
			}
		})
	}
	if ran == 0 {
		t.Fatalf("exportedAnnotationKnobs() knob-name format no longer matches the expected %q shape", "<TypeName>.<FieldName>")
	}
}
