package entapi

import (
	"testing"

	"entgo.io/ent/entc/gen"
)

func TestExtension_NewExtension(t *testing.T) {
	config := &ExtensionConfig{
		EntAPIPackage: "example.com/entapi",
	}

	ext := NewExtension(config)

	if ext.Config != config {
		t.Error("NewExtension should keep the config it was given")
	}
	if ext.Config.EntAPIPackage != "example.com/entapi" {
		t.Errorf("EntAPIPackage = %q, want %q", ext.Config.EntAPIPackage, "example.com/entapi")
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
		EntAPIPackage: "example.com/entapi",
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

	if configAnnotation.Config.EntAPIPackage != "example.com/entapi" {
		t.Errorf("annotation carries EntAPIPackage %q, want %q", configAnnotation.Config.EntAPIPackage, "example.com/entapi")
	}
}

func TestConfigAnnotation_Name(t *testing.T) {
	annotation := &ConfigAnnotation{}
	if annotation.Name() != "ExtensionConfig" {
		t.Errorf("Name() = %v, want %v", annotation.Name(), "ExtensionConfig")
	}
}

// TestNewExtensionWithOptions_NoOptions pins what is left of the option set:
// WithEntAPIPackage alone. WithBaseService and WithBaseHandler went with the
// templates they selected (#29), so calling NewExtensionWithOptions with
// nothing is the ordinary case rather than a degenerate one, and it must still
// produce a usable default configuration.
func TestNewExtensionWithOptions_NoOptions(t *testing.T) {
	ext := NewExtensionWithOptions()

	if ext.Config == nil {
		t.Fatal("NewExtensionWithOptions() returned a nil config")
	}
	if ext.Config.EntAPIPackage != defaultEntAPIPackage {
		t.Errorf("EntAPIPackage = %q, want the default %q", ext.Config.EntAPIPackage, defaultEntAPIPackage)
	}
}

func TestWithEntAPIPackage(t *testing.T) {
	// The default is the RUNTIME package, not this one (#15). Spelled as a
	// literal rather than as defaultEntAPIPackage, because comparing a
	// constant to itself would agree with any future edit — including one that
	// pointed generated code back at the generator, which is the regression
	// this pins.
	t.Run("default value is the runtime package", func(t *testing.T) {
		ext := NewExtension(nil)
		want := "github.com/githonllc/entapi/runtime"
		if ext.Config.EntAPIPackage != want {
			t.Errorf("default EntAPIPackage = %q, want %q — generated code that imports the "+
				"generator links the templates and the loader's init",
				ext.Config.EntAPIPackage, want)
		}
	})

	t.Run("custom value via WithEntAPIPackage", func(t *testing.T) {
		config := &ExtensionConfig{}
		opt := WithEntAPIPackage("custom/path")
		opt(config)

		if config.EntAPIPackage != "custom/path" {
			t.Errorf("EntAPIPackage = %q, want %q", config.EntAPIPackage, "custom/path")
		}
	})
}

func TestNewExtension_Defaults(t *testing.T) {
	ext := NewExtension(nil)

	if ext.Config.EntAPIPackage != "github.com/githonllc/entapi/runtime" {
		t.Errorf("EntAPIPackage = %q, want %q",
			ext.Config.EntAPIPackage, "github.com/githonllc/entapi/runtime")
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
	customPkg := "my/custom/entapi"
	ext := NewExtension(&ExtensionConfig{
		EntAPIPackage: customPkg,
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

	// Verify "entapiPkg" function exists and returns correct value
	entapiPkgFn, ok := funcMap["entapiPkg"]
	if !ok {
		t.Fatal("templateFuncMap() is missing the 'entapiPkg' function")
	}

	fn, ok := entapiPkgFn.(func() string)
	if !ok {
		t.Fatalf("entapiPkg has unexpected type %T, want func() string", entapiPkgFn)
	}
	got := fn()
	if got != customPkg {
		t.Errorf("entapiPkg() = %q, want %q", got, customPkg)
	}

	// Verify it does not mutate the global gen.Funcs map
	genFuncsBefore := make(map[string]bool, len(gen.Funcs))
	for k := range gen.Funcs {
		genFuncsBefore[k] = true
	}

	_ = ext.templateFuncMap()
	_ = ext.templateFuncMap()

	if _, exists := gen.Funcs["entapiPkg"]; exists {
		t.Error("templateFuncMap() mutated global gen.Funcs: found 'entapiPkg' key")
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

func TestNewExtensionWithOptions_EntAPIPackage(t *testing.T) {
	customPkg := "github.com/myorg/myentapi"
	ext := NewExtensionWithOptions(
		WithEntAPIPackage(customPkg),
	)

	if ext.Config.EntAPIPackage != customPkg {
		t.Errorf("EntAPIPackage = %q, want %q", ext.Config.EntAPIPackage, customPkg)
	}
}
