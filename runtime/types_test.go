package entdomain

import (
	"reflect"
	"strings"
	"testing"
)

// TestOrderIsDecidedInOnePlace closes the last of the "one rule, two spellings"
// residuals. Validating Order is Validate()'s only remaining job, so getting it
// wrong makes its whole remaining purpose wrong.
//
// SortKey sits on the only path into ListPage and reads Order with
// strings.EqualFold, so it is what actually decides the sort direction whether
// or not anyone validates. Validate() must therefore reject exactly what
// SortKey will not honour, and no more — the lenient side is authoritative, by
// the same argument that made Limit() authoritative over the page-size ceiling.
func TestOrderIsDecidedInOnePlace(t *testing.T) {
	allow := []string{"name"}

	t.Run("every spelling SortKey honours is also accepted by Validate", func(t *testing.T) {
		for _, c := range []struct {
			order    string
			wantDesc bool
		}{
			{"desc", true}, {"DESC", true}, {"Desc", true}, {"dEsC", true},
			{"asc", false}, {"ASC", false}, {"Asc", false},
			{"", false},
		} {
			r := ListRequest{SortBy: "name", Order: c.order}

			if err := r.Validate(); err != nil {
				t.Errorf("Order=%q: Validate() rejected a spelling SortKey honours: %v", c.order, err)
			}
			key, desc, err := r.SortKey(allow, "name")
			if err != nil {
				t.Errorf("Order=%q: SortKey() failed: %v", c.order, err)
				continue
			}
			if key != "name" || desc != c.wantDesc {
				t.Errorf("Order=%q: SortKey() = (%q, %v), want (\"name\", %v)", c.order, key, desc, c.wantDesc)
			}
		}
	})

	t.Run("a typo is rejected rather than silently read as ascending", func(t *testing.T) {
		for _, order := range []string{"descc", "sideways", "de sc", "-desc"} {
			r := ListRequest{SortBy: "name", Order: order}

			err := r.Validate()
			if err == nil {
				t.Errorf("Order=%q: Validate() accepted a value SortKey will not honour", order)
				continue
			}
			if !IsValidation(err) {
				t.Errorf("Order=%q: error must satisfy IsValidation, got %v", order, err)
			}
			// The reason it must be rejected: SortKey does not error on it, it
			// quietly returns ascending, so an unnoticed typo reverses results.
			if _, desc, sErr := r.SortKey(allow, "name"); sErr != nil || desc {
				t.Errorf("Order=%q: precondition changed — SortKey now reports (%v, %v)", order, desc, sErr)
			}
		}
	})
}

// TestPaginationTagsRestateNoRule: a struct tag cannot express EqualFold and
// cannot reference a constant, so any rule spelled there is a second spelling
// that can only drift. Validate(), Limit() and Offset() are the homes.
func TestPaginationTagsRestateNoRule(t *testing.T) {
	typ := reflect.TypeOf(ListRequest{})
	for _, name := range []string{"Size", "Page", "Order"} {
		f, ok := typ.FieldByName(name)
		if !ok {
			t.Fatalf("ListRequest has no %s field", name)
		}
		tag := f.Tag.Get("validate")
		for _, clause := range strings.Split(tag, ",") {
			switch clause = strings.TrimSpace(clause); {
			case clause == "", clause == "omitempty":
			default:
				t.Errorf("%s: validate tag %q restates a rule in %q; "+
					"a tag validator would then disagree with Validate()/Limit()/Offset()",
					name, tag, clause)
			}
		}
	}
}

func TestListRequestValidation(t *testing.T) {
	tests := []struct {
		name    string
		req     *ListRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: &ListRequest{
				Size:   10,
				Page:   0,
				SortBy: "name",
				Order:  "asc",
			},
			wantErr: false,
		},
		{
			name: "valid request with desc order",
			req: &ListRequest{
				Size:   20,
				Page:   10,
				SortBy: "created_at",
				Order:  "desc",
			},
			wantErr: false,
		},
		// Size and Page are no longer Validate()'s business: Limit() and
		// Offset() normalise them on the only path into ListPage, so rejecting
		// them here as well would be a second, bypassable reaction to the same
		// input. See TestClampingIsTheOnlyReactionToTheCeiling.
		{
			name: "negative limit is normalised, not rejected",
			req: &ListRequest{
				Size: -1,
				Page: 0,
			},
			wantErr: false,
		},
		{
			name: "negative page is normalised, not rejected",
			req: &ListRequest{
				Size: 10,
				Page: -1,
			},
			wantErr: false,
		},
		{
			name: "oversized limit is clamped, not rejected",
			req: &ListRequest{
				Size: 1001,
				Page: 0,
			},
			wantErr: false,
		},
		{
			name: "invalid order",
			req: &ListRequest{
				Size:  10,
				Page:  0,
				Order: "invalid",
			},
			wantErr: true,
		},
		{
			name:    "nil request",
			req:     nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ListRequest.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestZeroValueRequestNeedsNoPreparation replaces the old TestListRequestDefaults.
//
// The defect it encodes: SetDefaults() was a separate mutating call that
// nothing forced a caller to make, while Validate() accepted Size == 0. A
// handler that forgot the call passed a zero size straight through — the
// documented P0-8 in docs/QUALITY-REVIEW.md. The fix is not a reminder, it is
// removing the call that could be forgotten: Limit() defaults and clamps, and
// it sits on the only path into ListPage.
func TestZeroValueRequestNeedsNoPreparation(t *testing.T) {
	req := &ListRequest{}

	if err := req.Validate(); err != nil {
		t.Errorf("a zero-value request must be valid as-is: %v", err)
	}
	if got := req.Limit(); got != DefaultPageSize {
		t.Errorf("Limit() = %d, want %d without any preparatory call", got, DefaultPageSize)
	}
	if got := req.Offset(); got != 0 {
		t.Errorf("Offset() = %d, want 0", got)
	}
}
