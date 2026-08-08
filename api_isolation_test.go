package entapi

import "testing"

const schemaAPIPackage = "github.com/githonllc/entapi/api"

// TestSchemaAPIPackageIsGeneratorFree proves the schema annotation package is
// safe to import from an Ent schema without pulling generator or runtime code
// into the schema's dependency closure.
func TestSchemaAPIPackageIsGeneratorFree(t *testing.T) {
	t.Run("control/the probe detects entc in the generator", func(t *testing.T) {
		pkgs := listDeps(t, generatorPackage)
		if _, ok := pkgs["entgo.io/ent/entc/gen"]; !ok {
			t.Fatalf("the generator package %s does not reach entgo.io/ent/entc/gen; the absence probe is void", generatorPackage)
		}
	})

	pkgs := listDeps(t, schemaAPIPackage)
	if len(pkgs) == 0 {
		t.Fatalf("go list -deps %s returned nothing; the assertions below would pass vacuously", schemaAPIPackage)
	}
	for _, path := range []string{
		"entgo.io/ent/entc/gen",
		"golang.org/x/tools/imports",
		"embed",
		runtimePackage,
	} {
		if _, ok := pkgs[path]; ok {
			t.Errorf("schema API package %s reaches forbidden dependency %q", schemaAPIPackage, path)
		}
	}
}
