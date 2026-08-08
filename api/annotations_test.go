package api

import (
	"fmt"
	"reflect"
	"testing"

	"entgo.io/ent/schema"
)

func Example() {
	resource := Resource().Except(OpDelete)
	field := Searchable().Merge(Filterable()).(FieldAnnotation).Merge(Sortable()).(FieldAnnotation)
	edge := Expand().JSONKey("writer")

	fmt.Println(resource.Name(), resource.ExceptOps)
	fmt.Println(field.Name(), field.Searchable, field.Filterable, field.Sortable)
	fmt.Println(edge.Name(), edge.Expand, edge.Key)
	// Output:
	// EntAPIResource [delete]
	// EntAPIField true true true
	// EntAPIEdge true writer
}

func TestAnnotationNames(t *testing.T) {
	tests := []struct {
		annotation schema.Annotation
		want       string
	}{
		{Resource(), "EntAPIResource"},
		{Hidden(), "EntAPIField"},
		{Expand(), "EntAPIEdge"},
	}
	for _, tt := range tests {
		if got := tt.annotation.Name(); got != tt.want {
			t.Errorf("Name() = %q, want %q", got, tt.want)
		}
	}
}

func TestOpIsStringBacked(t *testing.T) {
	if got := reflect.TypeOf(OpCreate).Kind(); got != reflect.String {
		t.Fatalf("Op kind = %v, want string", got)
	}
	want := []Op{OpCreate, OpPatch, OpDelete, OpGet, OpList}
	for _, op := range want {
		if op == "" {
			t.Fatal("operation constants must serialize as non-empty words")
		}
	}
}

func TestResourceExceptReturnsACopy(t *testing.T) {
	base := Resource().Except(OpCreate)
	fork := base.Except(OpDelete)
	if got, want := base.ExceptOps, []Op{OpCreate}; !reflect.DeepEqual(got, want) {
		t.Fatalf("base ExceptOps = %v, want %v", got, want)
	}
	if got, want := fork.ExceptOps, []Op{OpCreate, OpDelete}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fork ExceptOps = %v, want %v", got, want)
	}
}

func TestEdgeJSONKeyReturnsACopy(t *testing.T) {
	base := Expand()
	fork := base.JSONKey("writer")
	if base.Key != "" {
		t.Fatalf("base Key = %q, want empty", base.Key)
	}
	if fork.Key != "writer" || !fork.Expand {
		t.Fatalf("fork = %+v, want expanded writer edge", fork)
	}
}

func TestFieldBuildersSetOneWord(t *testing.T) {
	tests := []struct {
		name string
		got  FieldAnnotation
		want FieldAnnotation
	}{
		{"Hidden", Hidden(), FieldAnnotation{Hidden: true}},
		{"ReadOnly", ReadOnly(), FieldAnnotation{ReadOnly: true}},
		{"Searchable", Searchable(), FieldAnnotation{Searchable: true}},
		{"Filterable", Filterable(), FieldAnnotation{Filterable: true}},
		{"Sortable", Sortable(), FieldAnnotation{Sortable: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !reflect.DeepEqual(tt.got, tt.want) {
				t.Fatalf("%s() = %+v, want %+v", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestMergers(t *testing.T) {
	t.Run("resource unions operations", func(t *testing.T) {
		got := Resource().Except(OpCreate).Merge(Resource().Except(OpDelete, OpCreate)).(ResourceAnnotation)
		if want := []Op{OpCreate, OpDelete}; !reflect.DeepEqual(got.ExceptOps, want) {
			t.Fatalf("ExceptOps = %v, want %v", got.ExceptOps, want)
		}
	})

	t.Run("field unions words", func(t *testing.T) {
		got := Searchable().Merge(Sortable()).(FieldAnnotation)
		if !got.Searchable || !got.Sortable {
			t.Fatalf("merged field = %+v, want Searchable and Sortable", got)
		}
	})

	t.Run("edge unions expand and keeps last non-empty JSON key", func(t *testing.T) {
		got := Expand().JSONKey("old").Merge(EdgeAnnotation{}).(EdgeAnnotation)
		if !got.Expand || got.Key != "old" {
			t.Fatalf("merge with empty key = %+v, want Expand and old key", got)
		}
		got = got.Merge(Expand().JSONKey("new")).(EdgeAnnotation)
		if !got.Expand || got.Key != "new" {
			t.Fatalf("merge with new key = %+v, want Expand and new key", got)
		}
	})
}

var (
	_ schema.Annotation = ResourceAnnotation{}
	_ schema.Annotation = FieldAnnotation{}
	_ schema.Annotation = EdgeAnnotation{}
	_ schema.Merger     = ResourceAnnotation{}
	_ schema.Merger     = FieldAnnotation{}
	_ schema.Merger     = EdgeAnnotation{}
)
