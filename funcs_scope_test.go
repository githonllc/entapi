package entapi

import (
	"reflect"
	"testing"

	"entgo.io/ent/entc/gen"

	"github.com/githonllc/entapi/api"
)

func TestAnnotationReadersAcceptTypedAndSerializedShapes(t *testing.T) {
	t.Run("resource", func(t *testing.T) {
		want := api.Resource().Except(api.OpCreate)
		for name, raw := range map[string]interface{}{
			"pointer": &want,
			"value":   want,
			"map":     map[string]interface{}{"except": []interface{}{"create"}},
		} {
			t.Run(name, func(t *testing.T) {
				node := &gen.Type{Annotations: gen.Annotations{resourceAnnotationName: raw}}
				if got := getResourceAnnotation(node); got == nil || !reflect.DeepEqual(*got, want) {
					t.Fatalf("getResourceAnnotation() = %#v, want %#v", got, want)
				}
			})
		}
	})

	t.Run("field", func(t *testing.T) {
		want := api.FieldAnnotation{Searchable: true, Sortable: true}
		for name, raw := range map[string]interface{}{
			"pointer": &want,
			"value":   want,
			"map":     map[string]interface{}{"searchable": true, "sortable": true},
		} {
			t.Run(name, func(t *testing.T) {
				field := newStringField("name", nil)
				field.Annotations = gen.Annotations{fieldAnnotationName: raw}
				if got := getFieldAnnotation(field); got == nil || !reflect.DeepEqual(*got, want) {
					t.Fatalf("getFieldAnnotation() = %#v, want %#v", got, want)
				}
			})
		}
	})

	t.Run("edge", func(t *testing.T) {
		want := api.Expand().JSONKey("owner")
		for name, raw := range map[string]interface{}{
			"pointer": &want,
			"value":   want,
			"map":     map[string]interface{}{"expand": true, "json_key": "owner"},
		} {
			t.Run(name, func(t *testing.T) {
				edge := &gen.Edge{Name: "author", Annotations: gen.Annotations{edgeAnnotationName: raw}}
				if got := getEdgeAnnotation(edge); got == nil || !reflect.DeepEqual(*got, want) {
					t.Fatalf("getEdgeAnnotation() = %#v, want %#v", got, want)
				}
			})
		}
	})
}

func TestAnnotationReadersRejectAbsentAndInvalidShapes(t *testing.T) {
	if got := getResourceAnnotation(nil); got != nil {
		t.Fatalf("nil resource = %#v", got)
	}
	if got := getFieldAnnotation(&gen.Field{Annotations: gen.Annotations{fieldAnnotationName: 42}}); got != nil {
		t.Fatalf("invalid field annotation = %#v", got)
	}
	if got := getEdgeAnnotation(&gen.Edge{}); got != nil {
		t.Fatalf("absent edge annotation = %#v", got)
	}
}

func TestHasCreateFamily(t *testing.T) {
	blocked := newStringField("secret", fieldPtr(api.Hidden()))
	node := newTestType("Account", blocked)
	if !hasCreateFamily(node) {
		t.Fatal("un-Excepted resource is rejected before templates and must retain the observable family state")
	}
	node.Annotations[resourceAnnotationName] = resourcePtr(api.Resource().Except(api.OpCreate))
	if hasCreateFamily(node) {
		t.Fatal("Except(OpCreate) must remove an unusable create family")
	}

	blocked.Optional = true
	if !hasCreateFamily(node) {
		t.Fatal("an Optional blocked field does not make create provably unusable")
	}
}
