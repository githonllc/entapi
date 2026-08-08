// This file is hand-written. Generation owns the package around it.
package httpdemoent

import (
	"os"
	"strings"
	"testing"
)

func TestExceptedDeleteFnAndOptionAreAbsentFromGeneratedSource(t *testing.T) {
	source, err := os.ReadFile("auditlog_handler.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{
		"type DeleteAuditLogFn",
		"func (f DeleteAuditLogFn) applyOption",
	} {
		if strings.Contains(string(source), absent) {
			t.Fatalf("auditlog_handler.go contains %q despite Except(api.OpDelete)", absent)
		}
	}
}
