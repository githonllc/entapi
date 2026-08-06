// This file is HAND-WRITTEN. Like orerr_contract_test.go in the edges fixture
// it is the exception to the rule that everything under
// internal/fixtures/<dir>/<dir>ent/ is generated: it carries no DO NOT EDIT header
// and TestCodegenFixtures does not rewrite it.
//
// It has to live in the generated package (`basicent`) because the thing it
// pins is the wire format
// of a generated type, and `go build` ignores test files, so the codegen
// harness is unaffected by its presence.
package basicent

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestListResponseWireFormat pins the JSON {Entity}ListResponse marshals to.
//
// #6 removed `PageInfo *entdomain.PageInfo` from it. The field was declared
// omitempty and nothing generated ever set it, so no payload on the wire ever
// carried a "pageInfo" key — which is exactly why a marshalling assertion alone
// cannot see the change.
//
// Since #65 the field-set half is pinned at compile time instead: the generated
// New{Entity}ListResponse converts an entdomain.Page, and a Go conversion is
// legal only while the two structs agree on field set, type and order — but a
// conversion ignores struct tags, so this test is what guards the JSON-tag half.
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
