package entapi

import (
	"reflect"
	"testing"
)

func TestListRequestV2Shape(t *testing.T) {
	typ := reflect.TypeOf(ListRequest{})
	want := map[string]reflect.Type{
		"Size": reflect.TypeOf(int(0)),
		"Page": reflect.TypeOf(int(0)),
		"Sort": reflect.TypeOf([]SortSpec(nil)),
	}
	if typ.NumField() != len(want) {
		t.Fatalf("ListRequest has %d fields, want exactly Size, Page, Sort", typ.NumField())
	}
	for name, wantType := range want {
		field, ok := typ.FieldByName(name)
		if !ok {
			t.Errorf("ListRequest has no %s field", name)
			continue
		}
		if field.Type != wantType {
			t.Errorf("ListRequest.%s type = %s, want %s", name, field.Type, wantType)
		}
		if got := field.Tag.Get("form"); got != "" {
			t.Errorf("ListRequest.%s retains retired form tag %q", name, got)
		}
	}
	for _, retired := range []string{"Sort" + "By", "Order"} {
		if _, ok := typ.FieldByName(retired); ok {
			t.Errorf("ListRequest.%s is still present", retired)
		}
	}
	ptrType := reflect.TypeOf(&ListRequest{})
	for _, retired := range []string{"Validate", "SortKey"} {
		if _, ok := ptrType.MethodByName(retired); ok {
			t.Errorf("ListRequest.%s is still present", retired)
		}
	}
}

func TestSortSpecCarriesKeyAndDirection(t *testing.T) {
	got := SortSpec{Key: "created_at", Desc: true}
	if got.Key != "created_at" || !got.Desc {
		t.Fatalf("SortSpec = %+v, want key created_at descending", got)
	}
}

func TestZeroValueRequestNeedsNoPreparation(t *testing.T) {
	req := ListRequest{}
	if got := req.Limit(); got != DefaultPageSize {
		t.Errorf("Limit() = %d, want %d without any preparatory call", got, DefaultPageSize)
	}
	if got := req.Offset(); got != 0 {
		t.Errorf("Offset() = %d, want 0", got)
	}
}
