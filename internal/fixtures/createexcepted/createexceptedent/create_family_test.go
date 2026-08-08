// This file is hand-written. Generation owns the package around it.
package createexceptedent

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestBrokenCreateFamilyIsAbsentAndPatchRemainsCallable(t *testing.T) {
	for _, name := range []string{"account_dto.go", "account_wiring.go"} {
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, absent := range []string{"type AccountCreateRequest", "type ValidAccountCreateRequest", "func CreateAccount"} {
			if strings.Contains(string(source), absent) {
				t.Errorf("%s contains %q; the unusable create family must be absent", name, absent)
			}
		}
	}

	_ = (func(context.Context, *Client, int, *ValidAccountPatchRequest) (*AccountResponse, error))(PatchAccount)
}
