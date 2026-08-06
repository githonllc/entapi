package entdomain

import (
	"go/parser"
	"go/token"
	"os"
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

// TestEveryEmbeddedTemplateIsLoaded asserts the converse of the walk above:
// being reachable through loadTemplate is not enough, template_index.go must
// actually bind the template to a package-level var. An embedded template that
// nothing loads looks live to a reader and to a grep, but edits to it silently
// do nothing — which is exactly how templates/model.tmpl came to be a stale
// byte-for-byte copy of templates/dto.tmpl.
func TestEveryEmbeddedTemplateIsLoaded(t *testing.T) {
	src, err := os.ReadFile("template_index.go")
	if err != nil {
		t.Fatalf("reading template_index.go failed: %v", err)
	}

	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		t.Fatalf("ReadDir(\"templates\") failed: %v", err)
	}

	checked := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tmpl") {
			continue
		}
		checked++
		base := strings.TrimSuffix(entry.Name(), ".tmpl")
		if !strings.Contains(string(src), `mustLoadTemplate("`+base+`")`) {
			t.Errorf("templates/%s is embedded but template_index.go never loads it; load it or delete the file", entry.Name())
		}
	}
	if checked == 0 {
		t.Fatal("no embedded templates were checked; this test would pass vacuously")
	}
}

// TestRemovedTemplatesStayRemoved pins #29's deletion in both halves.
//
// The templates are gone, but the FILE NAMES they used to produce are not
// forgotten: a consumer who generated with WithBaseService is holding two files
// per entity that this extension wrote and will never write again. Keeping the
// names in generatedFileNames is what makes the next generation run delete
// them, instead of leaving code that compiles against a service the library no
// longer describes. Dropping the names would turn the removal into a collision.
func TestRemovedTemplatesStayRemoved(t *testing.T) {
	for _, name := range []string{"base_service", "base_handler"} {
		if _, err := templateFS.ReadFile("templates/" + name + ".tmpl"); err == nil {
			t.Errorf("templates/%s.tmpl is still embedded; #29 removes the generated base service and handler", name)
		}
	}

	names := generatedFileNames(newTestType("Widget"))
	index := make(map[string]bool, len(names))
	for _, n := range names {
		index[n] = true
	}
	for _, legacy := range []string{"widget_base_service.go", "widget_base_handler.go"} {
		if !index[legacy] {
			t.Errorf("generatedFileNames() no longer lists %q; removeStaleArtifacts is the only thing that deletes it from a consumer's tree, so the name has to outlive the template", legacy)
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
