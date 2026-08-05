package entdomain

import (
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// TestEmbeddedTemplatesLoadBySlashPath walks the embedded templates directory and
// asserts that every entry it finds is reachable through loadTemplate by its base
// name. The listing is read from the embedded FS rather than hardcoded, so a new
// template that is added but not loadable fails here too.
func TestEmbeddedTemplatesLoadBySlashPath(t *testing.T) {
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		t.Fatalf("ReadDir(\"templates\") failed: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("embedded templates directory is empty")
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".tmpl") {
			continue
		}
		base := strings.TrimSuffix(name, ".tmpl")

		// The embedded FS only ever accepts slash-separated paths, so the path
		// loadTemplate builds must be reachable through the same slash form.
		if _, err := templateFS.ReadFile("templates/" + name); err != nil {
			t.Errorf("templates/%s not readable by slash path: %v", name, err)
		}
		content, err := loadTemplate(base)
		if err != nil {
			t.Errorf("loadTemplate(%q) failed: %v", base, err)
			continue
		}
		if content == "" {
			t.Errorf("loadTemplate(%q) returned empty content", base)
		}
	}
}

// TestTemplateLoaderDoesNotUsePathFilepath pins the platform contract that the
// directory walk above cannot observe on a slash-separated host: on darwin and
// linux filepath.Join produces the same string as path.Join, so only on Windows
// would the lookup actually break. Asserting the loader does not import
// path/filepath keeps the separator-aware package out of embedded-path
// construction on every host.
func TestTemplateLoaderDoesNotUsePathFilepath(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "template_loader.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parsing template_loader.go failed: %v", err)
	}
	for _, imp := range file.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			t.Fatalf("unquoting import %s failed: %v", imp.Path.Value, err)
		}
		if p == "path/filepath" {
			t.Error("template_loader.go imports \"path/filepath\"; embedded FS paths are always slash-separated and must be built with \"path\"")
		}
	}
}
