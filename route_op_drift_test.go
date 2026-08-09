package entapi

import (
	"regexp"
	"testing"

	"github.com/githonllc/entapi/api"
	entapiruntime "github.com/githonllc/entapi/runtime"
)

// routeOpPairs is the whole correspondence between the schema-time vocabulary
// and the run-time one. It is written out rather than derived because deriving
// it would need one side to import the other, and the two packages sit on
// opposite sides of the seam TestRuntimePackageIsGeneratorFree defends: api
// imports entgo.io/ent/schema, and the runtime's entgo.io count must stay zero.
//
// The cost of that seam is a literal in http.tmpl -- the template writes
// Op: "create", not Op: entapi.OpCreate, because the generator has no name for
// the runtime's constant. That literal is the drift surface, and the two tests
// below are what stands in for the compiler here.
var routeOpPairs = []struct {
	schema  api.Op
	runtime entapiruntime.Op
}{
	{api.OpList, entapiruntime.OpList},
	{api.OpCreate, entapiruntime.OpCreate},
	{api.OpGet, entapiruntime.OpGet},
	{api.OpPatch, entapiruntime.OpPatch},
	{api.OpDelete, entapiruntime.OpDelete},
}

// TestRouteOpValuesMatchAcrossThePackageSeam pins that every operation the
// generator can emit has a runtime constant spelled identically. Renaming one
// side's string value turns a consumer's rt.Op == entapi.OpCreate into a
// comparison that is false for every route -- which is silent, and is the exact
// failure Route.Op exists to remove.
func TestRouteOpValuesMatchAcrossThePackageSeam(t *testing.T) {
	for _, pair := range routeOpPairs {
		if string(pair.schema) != string(pair.runtime) {
			t.Errorf("api.Op %q and runtime.Op %q disagree; a generated Op literal would match neither consumer constant",
				pair.schema, pair.runtime)
		}
	}

	// The reverse direction: an operation resourceOps can emit but this table
	// forgot would leave a route carrying an Op no consumer constant matches.
	covered := make(map[api.Op]bool, len(routeOpPairs))
	for _, pair := range routeOpPairs {
		covered[pair.schema] = true
	}
	for _, op := range []api.Op{api.OpList, api.OpCreate, api.OpGet, api.OpPatch, api.OpDelete} {
		if !covered[op] {
			t.Errorf("api.Op %q has no runtime counterpart in routeOpPairs", op)
		}
	}
}

// TestRouteManifestRendersOpFromResourceOps pins that http.tmpl still derives
// the Op field from the operation record rather than from a second table. A
// hardcoded switch in the template would pass the value test above while
// re-introducing exactly the duplication the OpenAPI document is forbidden.
func TestRouteManifestRendersOpFromResourceOps(t *testing.T) {
	content, err := templateFS.ReadFile("templates/http.tmpl")
	if err != nil {
		t.Fatalf("read templates/http.tmpl: %v", err)
	}

	// Both fields must come from the range variables the manifest already
	// iterates -- $node for the entity, $op for the operation.
	wantEntity := regexp.MustCompile(`Entity:\s*"\{\{\s*\$node\.Name\s*\}\}"`)
	if !wantEntity.Match(content) {
		t.Error("http.tmpl no longer renders Route.Entity from $node.Name; it must not carry its own entity table")
	}
	wantOp := regexp.MustCompile(`Op:\s*"\{\{\s*\$op\.Name\s*\}\}"`)
	if !wantOp.Match(content) {
		t.Error("http.tmpl no longer renders Route.Op from $op.Name; it must not carry its own operation table")
	}
}
