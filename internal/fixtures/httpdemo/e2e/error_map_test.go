package e2e

import (
	"strings"

	ent "github.com/githonllc/entapi/internal/fixtures/httpdemo/httpdemoent"
)

func init() {
	ent.ErrorMap = ent.ErrorMap.WithUniqueViolation(func(err error) bool {
		return strings.Contains(err.Error(), "UNIQUE constraint failed")
	})
}
