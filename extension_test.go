package entdomain

import (
	"testing"

	"entgo.io/ent/entc/gen"
)

func TestExtension_NewExtension(t *testing.T) {
	config := &ExtensionConfig{
		EntDomainPackage: "example.com/entdomain",
	}

	ext := NewExtension(config)

	if ext.Config != config {
		t.Error("NewExtension should keep the config it was given")
	}
	if ext.Config.EntDomainPackage != "example.com/entdomain" {
		t.Errorf("EntDomainPackage = %q, want %q", ext.Config.EntDomainPackage, "example.com/entdomain")
	}
}

func TestExtension_Templates(t *testing.T) {
	ext := NewExtension(nil)

	templates := ext.Templates()

	// Extension uses Hook-based generation, Templates() should return empty slice
	if len(templates) != 0 {
		t.Errorf("Expected 0 templates (Hook-based generation), got %d", len(templates))
	}
}

func TestExtension_Options(t *testing.T) {
	ext := NewExtension(&ExtensionConfig{})

	options := ext.Options()
	if options == nil {
		t.Error("Options should not be nil")
	}
}

func TestExtension_Annotations(t *testing.T) {
	config := &ExtensionConfig{
		EntDomainPackage: "example.com/entdomain",
	}
	ext := NewExtension(config)
	annotations := ext.Annotations()

	if len(annotations) != 1 {
		t.Errorf("Expected 1 annotation, got %d", len(annotations))
	}

	configAnnotation, ok := annotations[0].(*ConfigAnnotation)
	if !ok {
		t.Fatalf("Expected *ConfigAnnotation, got %T", annotations[0])
	}

	if configAnnotation.Config.EntDomainPackage != "example.com/entdomain" {
		t.Errorf("annotation carries EntDomainPackage %q, want %q", configAnnotation.Config.EntDomainPackage, "example.com/entdomain")
	}
}

func TestConfigAnnotation_Name(t *testing.T) {
	annotation := &ConfigAnnotation{}
	if annotation.Name() != "ExtensionConfig" {
		t.Errorf("Name() = %v, want %v", annotation.Name(), "ExtensionConfig")
	}
}

// TestNewExtensionWithOptions_NoOptions pins what is left of the option set:
// WithEntDomainPackage alone. WithBaseService and WithBaseHandler went with the
// templates they selected (#29), so calling NewExtensionWithOptions with
// nothing is the ordinary case rather than a degenerate one, and it must still
// produce a usable default configuration.
func TestNewExtensionWithOptions_NoOptions(t *testing.T) {
	ext := NewExtensionWithOptions()

	if ext.Config == nil {
		t.Fatal("NewExtensionWithOptions() returned a nil config")
	}
	if ext.Config.EntDomainPackage != defaultEntDomainPackage {
		t.Errorf("EntDomainPackage = %q, want the default %q", ext.Config.EntDomainPackage, defaultEntDomainPackage)
	}
}

func TestWithEntDomainPackage(t *testing.T) {
	t.Run("default value", func(t *testing.T) {
		ext := NewExtension(nil)
		want := "github.com/githonllc/entdomain"
		if ext.Config.EntDomainPackage != want {
			t.Errorf("default EntDomainPackage = %q, want %q", ext.Config.EntDomainPackage, want)
		}
	})

	t.Run("custom value via WithEntDomainPackage", func(t *testing.T) {
		config := &ExtensionConfig{}
		opt := WithEntDomainPackage("custom/path")
		opt(config)

		if config.EntDomainPackage != "custom/path" {
			t.Errorf("EntDomainPackage = %q, want %q", config.EntDomainPackage, "custom/path")
		}
	})
}

func TestNewExtension_Defaults(t *testing.T) {
	ext := NewExtension(nil)

	if ext.Config.EntDomainPackage != "github.com/githonllc/entdomain" {
		t.Errorf("EntDomainPackage = %q, want %q", ext.Config.EntDomainPackage, "github.com/githonllc/entdomain")
	}
}

func TestExtension_Hooks(t *testing.T) {
	ext := NewExtension(nil)
	hooks := ext.Hooks()

	if len(hooks) != 1 {
		t.Errorf("Hooks() returned %d hooks, want exactly 1", len(hooks))
	}
}

func TestExtension_TemplateFuncMap(t *testing.T) {
	customPkg := "my/custom/entdomain"
	ext := NewExtension(&ExtensionConfig{
		EntDomainPackage: customPkg,
	})

	funcMap := ext.templateFuncMap()

	// Verify that gen.Funcs entries are included
	for key := range gen.Funcs {
		if _, ok := funcMap[key]; !ok {
			t.Errorf("templateFuncMap() is missing gen.Funcs key %q", key)
		}
	}

	// Verify that custom templateFuncs entries are included
	customKeys := templateFuncs()
	for key := range customKeys {
		if _, ok := funcMap[key]; !ok {
			t.Errorf("templateFuncMap() is missing custom templateFuncs key %q", key)
		}
	}

	// Verify "entdomainPkg" function exists and returns correct value
	entdomainPkgFn, ok := funcMap["entdomainPkg"]
	if !ok {
		t.Fatal("templateFuncMap() is missing the 'entdomainPkg' function")
	}

	fn, ok := entdomainPkgFn.(func() string)
	if !ok {
		t.Fatalf("entdomainPkg has unexpected type %T, want func() string", entdomainPkgFn)
	}
	got := fn()
	if got != customPkg {
		t.Errorf("entdomainPkg() = %q, want %q", got, customPkg)
	}

	// Verify it does not mutate the global gen.Funcs map
	genFuncsBefore := make(map[string]bool, len(gen.Funcs))
	for k := range gen.Funcs {
		genFuncsBefore[k] = true
	}

	_ = ext.templateFuncMap()
	_ = ext.templateFuncMap()

	if _, exists := gen.Funcs["entdomainPkg"]; exists {
		t.Error("templateFuncMap() mutated global gen.Funcs: found 'entdomainPkg' key")
	}

	for k := range gen.Funcs {
		if !genFuncsBefore[k] {
			t.Errorf("templateFuncMap() added unexpected key %q to global gen.Funcs", k)
		}
	}
}

func TestConfigAnnotation_NameRenamed(t *testing.T) {
	annotation := ConfigAnnotation{}
	got := annotation.Name()
	want := "ExtensionConfig"
	if got != want {
		t.Errorf("ConfigAnnotation.Name() = %q, want %q (was renamed from DomainConfig)", got, want)
	}
}

func TestNewExtensionWithOptions_EntDomainPackage(t *testing.T) {
	customPkg := "github.com/myorg/myentdomain"
	ext := NewExtensionWithOptions(
		WithEntDomainPackage(customPkg),
	)

	if ext.Config.EntDomainPackage != customPkg {
		t.Errorf("EntDomainPackage = %q, want %q", ext.Config.EntDomainPackage, customPkg)
	}
}
