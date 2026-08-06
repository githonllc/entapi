// This file is HAND-WRITTEN. Like orerr_contract_test.go in the edges fixture
// it is the exception to the rule that everything under
// internal/fixtures/<dir>/ent/ is generated: it carries no DO NOT EDIT header
// and TestCodegenFixtures does not rewrite it.
//
// It has to live in `package ent` because the thing it pins is the wire format
// of a generated type, and `go build` ignores test files, so the codegen
// harness is unaffected by its presence.
package ent

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestListResponseWireFormat pins the JSON {Entity}ListResponse marshals to.
//
// #6 removed `PageInfo *entdomain.PageInfo` from it. The field was declared
// omitempty and nothing generated ever set it, so no payload on the wire ever
// carried a "pageInfo" key — which is exactly why a marshalling assertion alone
// cannot see the change, and why the field-set assertion below exists too.
func TestListResponseWireFormat(t *testing.T) {
	id := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	created := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	resp := WidgetListResponse{
		Data:  []*WidgetResponse{{ID: id, Name: "widget", CreatedAt: created}},
		Total: 3,
		Page:  1,
		Size:  20,
	}

	encoded, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	const want = `{"data":[{"id":"11111111-2222-3333-4444-555555555555","name":"widget","created_at":"2024-01-02T03:04:05Z"}],"total":3,"page":1,"size":20}`
	if got := string(encoded); got != want {
		t.Errorf("WidgetListResponse marshals to\n  %s\nwant\n  %s", got, want)
	}
}

// TestListResponseCarriesNoPageInfo is the half that actually observes #6.
//
// The struct field is what was published, not the payload: a consumer could
// name the type, read `.PageInfo`, or unmarshal into it. Asserting on the
// declared field set is therefore the assertion that matches the breaking
// change, and it is checked by name and by tag so a rename cannot smuggle it
// back.
func TestListResponseCarriesNoPageInfo(t *testing.T) {
	rt := reflect.TypeOf(WidgetListResponse{})

	if _, found := rt.FieldByName("PageInfo"); found {
		t.Error("WidgetListResponse.PageInfo is back; the cursor metadata it carried " +
			"has no producer — the generated wiring returns entdomain.Page[…]")
	}

	var got []string
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		got = append(got, name)
	}
	want := []string{"data", "total", "page", "size"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("WidgetListResponse json fields = %v, want %v", got, want)
	}
}
