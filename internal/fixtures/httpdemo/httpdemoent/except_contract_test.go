// This file is hand-written. Generation owns the package around it.
package httpdemoent

import (
	"os"
	"strings"
	"testing"
)

func TestExceptedDeleteFnIsAbsentFromGeneratedSource(t *testing.T) {
	source, err := os.ReadFile("auditlog_handler.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(source), "DeleteAuditLogFn") {
		t.Fatal("auditlog_handler.go contains DeleteAuditLogFn despite Except(api.OpDelete)")
	}
}
