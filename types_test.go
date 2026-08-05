package entdomain

import (
	"testing"
)

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
// documented P0-8 in QUALITY-REVIEW.md. The fix is not a reminder, it is
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
