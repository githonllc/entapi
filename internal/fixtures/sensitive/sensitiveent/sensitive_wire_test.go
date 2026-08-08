// This file is HAND-WRITTEN. TestCodegenFixtures generates the package around
// it but does not replace this wire-format contract.
package sensitiveent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestSensitiveFieldsAreAbsentFromResponseWireFormat proves absence on the
// wire, both directly and through an expanded edge's summary.
func TestSensitiveFieldsAreAbsentFromResponseWireFormat(t *testing.T) {
	id := uuid.MustParse("11111111-2222-3333-4444-555555555555")

	tests := []struct {
		name  string
		value any
	}{
		{
			name:  "account response",
			value: AccountResponse{ID: id, Name: "Ada"},
		},
		{
			name: "expanded account summary",
			value: SessionResponse{
				ID:        id,
				UserAgent: "fixture",
				Account:   &AccountSummary{ID: id, Name: "Ada"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := json.Marshal(tt.value)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			for _, key := range []string{"password_hash", "login_window"} {
				if strings.Contains(string(encoded), `"`+key+`"`) {
					t.Errorf("sensitive key %q is present in %s", key, encoded)
				}
			}
		})
	}
}
