package entapi

import (
	"bytes"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
)

const softDeleteConfigInjection = `	// entapi: soft-delete hooks and interceptors installed by newConfig.
	cfg.hooks.Doc = append(cfg.hooks.Doc, softDeleteHook())
	cfg.inters.Doc = append(cfg.inters.Doc, softDeleteTraverser())`

// TestSoftDeleteConfigInjectionGuard pins the undocumented ent partial
// extension point this feature relies on. If an ent upgrade stops executing
// config/init/fields/* inside newConfig, generation can still succeed while
// every client silently falls back to hard delete; this assertion must fail.
func TestSoftDeleteConfigInjectionGuard(t *testing.T) {
	root := repoRoot(t)
	target := t.TempDir()
	if err := generateFixture(
		"softdelete",
		fixtureSchemaDir(root, "softdelete"),
		target,
		nil,
		[]gen.Feature{gen.FeaturePrivacy},
	); err != nil {
		t.Fatalf("generate softdelete fixture: %v", err)
	}

	client := readGeneratedFile(t, target, "client.go")
	if !strings.Contains(string(client), softDeleteConfigInjection) {
		t.Fatalf("generated client.go does not contain the soft-delete newConfig injection marker and assignments:\n%s", client)
	}
	softDelete := readGeneratedFile(t, target, softDeleteFileName)
	if strings.Contains(string(softDelete), "RegisterSoftDelete") {
		t.Fatal("generated entapi_softdelete.go still declares the removed RegisterSoftDelete ritual")
	}
}

// TestSoftDeleteConfigPartialIsByteCleanWithoutMixin compares the only output
// the partial can alter against plain ent generation. Rendering no marker is
// not enough: whitespace alone would still rewrite every consumer client.go.
func TestSoftDeleteConfigPartialIsByteCleanWithoutMixin(t *testing.T) {
	root := repoRoot(t)
	schemaDir := fixtureSchemaDir(root, "basic")
	pkgPath := fixtureEntPkgPath("basic")
	plainTarget := t.TempDir()
	extendedTarget := t.TempDir()

	if err := entc.Generate(schemaDir, &gen.Config{Target: plainTarget, Package: pkgPath}); err != nil {
		t.Fatalf("plain ent generation: %v", err)
	}
	if err := generateFixture("basic", schemaDir, extendedTarget, nil, nil); err != nil {
		t.Fatalf("entapi generation: %v", err)
	}

	plain := readGeneratedFile(t, plainTarget, "client.go")
	extended := readGeneratedFile(t, extendedTarget, "client.go")
	if !bytes.Equal(plain, extended) {
		t.Fatalf("client.go changed for a graph without SoftDeleteMixin: plain sha256=%x, entapi sha256=%x",
			sha256.Sum256(plain), sha256.Sum256(extended))
	}
}

func readGeneratedFile(t *testing.T, target, name string) []byte {
	t.Helper()
	path := filepath.Join(target, name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated %s: %v", path, err)
	}
	return b
}
