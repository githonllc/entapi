package entapi

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
	"golang.org/x/tools/imports"
)

// Extension is the entapi Ent extension.
type Extension struct {
	// Config holds the extension configuration.
	Config *ExtensionConfig
}

// ExtensionConfig holds configuration for the extension.
//
// GenerateBaseService and GenerateBaseHandler used to live here. Both are gone
// with the templates they selected (#29); every generated artifact is now
// emitted unconditionally for an api.Resource, so there is nothing left to
// switch on. See the migration note in README.md.
type ExtensionConfig struct {
	// EntAPIPackage is the import path generated code imports for the Layer 2
	// runtime — ErrValidation, ListRequest, ListPage, the error mapper.
	// Default: "github.com/githonllc/entapi/runtime"
	EntAPIPackage string

	// StrictQueryOperators makes an unrecognised operator prefix a validation
	// failure instead of falling back to whole-value equality. It is off by
	// default because bare RFC-3339 timestamps rely on that fallback.
	StrictQueryOperators bool

	// OpenAPITitle is info.title of the generated openapi.yaml.
	// Default: the ent package name plus " API", e.g. "ent API".
	OpenAPITitle string

	// OpenAPIVersion is info.version of the generated openapi.yaml.
	// Default: defaultOpenAPIVersion.
	//
	// It is generation-time configuration rather than something read from the
	// working tree. Deriving it from a git tag would make the document a
	// function of state no other generated file depends on, and a clean
	// checkout would stop staying clean across a test run.
	OpenAPIVersion string
}

// defaultEntAPIPackage is the RUNTIME package, not this one (#15).
//
// The two were the same string until the runtime types moved out, and that is
// what put the embedded templates and the template loader's init into every
// consumer's production binary: generated code imported a sentinel error and
// linked the generator. They are now different packages, and
// TestRuntimePackageIsGeneratorFree is what keeps them apart.
//
// The runtime package is still named `entapi`, so this changes the import
// LINE in generated output and nothing else — every `entapi.X` reference in
// every generated file and every consumer call site is unaffected.
const defaultEntAPIPackage = "github.com/githonllc/entapi/runtime"

// NewExtension creates a new extension instance
func NewExtension(config *ExtensionConfig) *Extension {
	if config == nil {
		config = &ExtensionConfig{}
	}
	if config.EntAPIPackage == "" {
		config.EntAPIPackage = defaultEntAPIPackage
	}

	return &Extension{
		Config: config,
	}
}

// Hooks returns the extension's hooks — uses a Hook to generate separate files per Type.
func (e *Extension) Hooks() []gen.Hook {
	return []gen.Hook{
		e.generatePerTypeFiles, // main generation logic
	}
}

// Templates returns the partial that extends ent's own newConfig body.
//
// It is not a standalone GraphTemplate output: its config/init/fields/* name
// makes ent splice it into client.go. The partial renders nothing for a graph
// without SoftDeleteMixin, leaving that graph's client.go byte-identical to
// ordinary ent output.
func (e *Extension) Templates() []*gen.Template {
	return []*gen.Template{
		gen.MustParse(gen.NewTemplate("entapi_softdelete_config_init").
			Funcs(e.templateFuncMap()).
			Parse(softDeleteConfigInitTemplate)),
	}
}

// perTypeFileName is the file one per-type template's output for node lands in:
// ent/{entity}_dto.go, _filter.go, _wiring.go and _handler.go.
//
// One function rather than three format strings, because the refusal messages in
// schema_conflicts.go quote these names back to a schema author (#62). A message
// naming a file the generator does not actually write is worse than no file name
// at all — it sends the author looking for something that is not there.
func perTypeFileName(node *gen.Type, kind string) string {
	return fmt.Sprintf("%s_%s.go", strings.ToLower(node.Name), kind)
}

// pendingFile is one rendered, formatted file waiting to be written. Holding it
// in memory is what makes the run atomic: see generatePerTypeFiles.
type pendingFile struct {
	path    string
	content []byte
}

// generatePerTypeFiles is the core Hook that generates separate files for each Type.
//
// Generation runs in two phases, and the split is the whole atomicity story
// (#61, docs/adr/0003-per-run-atomic-generation.md). Phase 1 renders AND
// formats every file of the run into memory; phase 2 writes them. Because
// imports.Process is a pure function of the bytes, every deterministic failure
// — a template bug, a missing import, a refused schema — is raised in phase 1,
// with not a single path on disk touched. The previous run's output survives a
// failed run intact and entire, rather than being replaced up to the entity
// that failed.
//
// What remains is the phase-2 rename loop. A hard kill (SIGKILL, power loss)
// between two renames still leaves a mix of two generations. That window is
// milliseconds wide and unreachable by any failure the process survives, and
// closing it would need a directory swap that is not atomic across platforms;
// ADR-0003 accepts it deliberately. Do not read "atomic per run" as more than
// that.
func (e *Extension) generatePerTypeFiles(next gen.Generator) gen.Generator {
	return gen.GenerateFunc(func(g *gen.Graph) error {
		// Reject annotations that contradict the ent schema before anything is
		// written — including by ent's own generator, which runs below. A
		// contradiction cannot be generated into code that compiles, so the
		// only honest outcomes are a clear error here or a compile error in the
		// consumer's package. See schema_conflicts.go.
		if err := checkGraphConflicts(g); err != nil {
			return err
		}

		// Run the standard generation first
		if err := next.Generate(g); err != nil {
			return err
		}

		// PHASE 1 — render and format everything, write nothing.
		//
		// Separate files are produced for each api.Resource. Other entities are
		// skipped completely.
		//
		// written records this run's output set, which is what tells a file
		// left over from an earlier run apart from one just produced.
		var pending []pendingFile
		written := make(map[string]bool)
		wiredAny := false
		for _, node := range g.Nodes {
			if !isResource(node) {
				continue
			}
			wiredAny = true

			// DTO file → ent/{entity}_dto.go
			file, err := e.renderDTOFile(g, node)
			if err != nil {
				return fmt.Errorf("failed to generate %s DTO: %w", node.Name, err)
			}
			pending = append(pending, file)
			written[file.path] = true

			// The query surface → ent/{entity}_filter.go
			file, err = e.renderFilterFile(g, node)
			if err != nil {
				return fmt.Errorf("failed to generate %s filter: %w", node.Name, err)
			}
			pending = append(pending, file)
			written[file.path] = true

			// The wiring → ent/{entity}_wiring.go
			file, err = e.renderWiringFile(g, node)
			if err != nil {
				return fmt.Errorf("failed to generate %s wiring: %w", node.Name, err)
			}
			pending = append(pending, file)
			written[file.path] = true

			// The three-step HTTP handlers → ent/{entity}_handler.go
			file, err = e.renderHandlerFile(g, node)
			if err != nil {
				return fmt.Errorf("failed to generate %s handlers: %w", node.Name, err)
			}
			pending = append(pending, file)
			written[file.path] = true
		}

		// The error classifier is generated once per GRAPH for the reason the
		// wiring needs it to be: every entity's wiring lands in the one
		// package, so one declaration serves all of them — and that is what
		// makes a not-found read the same way whichever operation produced it.
		//
		// It is written only when at least one entity produced wiring. Nothing
		// else would reference it, and an ErrorMap alone in a package is a
		// public name this extension cannot explain.
		if wiredAny {
			file, err := e.renderErrorMapFile(g)
			if err != nil {
				return fmt.Errorf("failed to generate the error classifier: %w", err)
			}
			pending = append(pending, file)
			written[file.path] = true

			file, err = e.renderHTTPFile(g)
			if err != nil {
				return fmt.Errorf("failed to generate the HTTP route tree: %w", err)
			}
			pending = append(pending, file)
			written[file.path] = true

			// The document and the file that embeds it are gated together, on
			// the same condition as the route tree they describe. Splitting
			// them would let entapi_openapi.go embed a path that is not there.
			file, err = e.renderOpenAPIFile(g)
			if err != nil {
				return fmt.Errorf("failed to generate the OpenAPI document: %w", err)
			}
			pending = append(pending, file)
			written[file.path] = true

			file, err = e.renderOpenAPIEmbedFile(g)
			if err != nil {
				return fmt.Errorf("failed to generate the OpenAPI embed: %w", err)
			}
			pending = append(pending, file)
			written[file.path] = true
		}

		// The soft-delete traverser is generated once per GRAPH, not per type:
		// it is one type switch over the entities embedding
		// entapi.SoftDeleteMixin. It sits outside the loop above for a
		// second reason too — that loop skips a node with no domain fields,
		// and soft delete is a property of the ent schema rather than of the
		// HTTP surface, so an entity with no annotated field at all still has
		// to be filtered.
		softDelete, err := e.renderSoftDeleteFile(g)
		if err != nil {
			return fmt.Errorf("failed to generate the soft-delete traverser: %w", err)
		}
		if softDelete.path != "" {
			pending = append(pending, softDelete)
			written[softDelete.path] = true
		}

		// PHASE 2 — write. Everything above succeeded, so the only errors left
		// here are the ones no amount of staging can pre-empt: ENOSPC, EACCES,
		// a target directory that vanished mid-run.
		for _, file := range pending {
			if err := writeFormatted(file.path, file.content); err != nil {
				return err
			}
		}

		// Only once every file is on disk: a run that failed partway must not
		// delete anything, or a template bug would take the previous output
		// with it.
		if err := removeStaleArtifacts(g.Config.Target, written); err != nil {
			return err
		}

		return nil
	})
}

// renderDTOFile renders and formats the DTO file for a single Type. It does not
// write: see generatePerTypeFiles for why every render*File stops at memory.
// Output: ent/{entity}_dto.go
func (e *Extension) renderDTOFile(g *gen.Graph, node *gen.Type) (pendingFile, error) {
	tmpl, err := template.New("dto").
		Funcs(e.templateFuncMap()).
		Parse(dtoTemplate)
	if err != nil {
		return pendingFile{}, fmt.Errorf("failed to parse DTO template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, node); err != nil {
		return pendingFile{}, fmt.Errorf("failed to render DTO template: %w", err)
	}

	outputPath := filepath.Join(g.Config.Target, perTypeFileName(node, "dto"))

	formatted, err := formatFile(outputPath, buf.Bytes())
	if err != nil {
		return pendingFile{}, err
	}
	return pendingFile{path: outputPath, content: formatted}, nil
}

// renderFilterFile renders and formats the query surface for a single Type: the
// filter struct, its predicates and the sort allow-list.
// Output: ent/{entity}_filter.go
//
// It is emitted for every annotated entity, including one that marks no field
// filterable, searchable or sortable. Such an entity gets an empty filter type
// and an empty allow-list, which is the honest answer — and the safe one, since
// an empty allow-list makes the entity orderable by nothing at all.
func (e *Extension) renderFilterFile(g *gen.Graph, node *gen.Type) (pendingFile, error) {
	tmpl, err := template.New("filter").
		Funcs(e.templateFuncMap()).
		Parse(filterTemplate)
	if err != nil {
		return pendingFile{}, fmt.Errorf("failed to parse filter template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, node); err != nil {
		return pendingFile{}, fmt.Errorf("failed to render filter template: %w", err)
	}

	outputPath := filepath.Join(g.Config.Target, perTypeFileName(node, "filter"))

	formatted, err := formatFile(outputPath, buf.Bytes())
	if err != nil {
		return pendingFile{}, err
	}
	return pendingFile{path: outputPath, content: formatted}, nil
}

// renderWiringFile renders and formats the wiring for a single Type: one free
// function per operation, each handing this entity's generated artifacts to a
// routine in the runtime.
// Output: ent/{entity}_wiring.go
//
// It is emitted unconditionally, like the filter file and for the same reason:
// every entity the generator handles has a response type, an eager-load plan, a
// filter and a sort allow-list, so every one of them can be read, listed and
// deleted. The create and update functions are the two that depend on the
// scopes — an entity with no create-scoped field gets no Create.
func (e *Extension) renderWiringFile(g *gen.Graph, node *gen.Type) (pendingFile, error) {
	tmpl, err := template.New("wiring").
		Funcs(e.templateFuncMap()).
		Parse(wiringTemplate)
	if err != nil {
		return pendingFile{}, fmt.Errorf("failed to parse wiring template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, node); err != nil {
		return pendingFile{}, fmt.Errorf("failed to render wiring template: %w", err)
	}

	outputPath := filepath.Join(g.Config.Target, perTypeFileName(node, "wiring"))

	formatted, err := formatFile(outputPath, buf.Bytes())
	if err != nil {
		return pendingFile{}, err
	}
	return pendingFile{path: outputPath, content: formatted}, nil
}

// renderHandlerFile renders and formats the three-step HTTP handlers for one
// Resource. Output: ent/{entity}_handler.go
func (e *Extension) renderHandlerFile(g *gen.Graph, node *gen.Type) (pendingFile, error) {
	tmpl, err := template.New("handler").
		Funcs(e.templateFuncMap()).
		Parse(handlerTemplate)
	if err != nil {
		return pendingFile{}, fmt.Errorf("failed to parse handler template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, node); err != nil {
		return pendingFile{}, fmt.Errorf("failed to render handler template: %w", err)
	}

	outputPath := filepath.Join(g.Config.Target, perTypeFileName(node, "handler"))
	formatted, err := formatFile(outputPath, buf.Bytes())
	if err != nil {
		return pendingFile{}, err
	}
	return pendingFile{path: outputPath, content: formatted}, nil
}

// renderErrorMapFile renders and formats the package-level error classifier the
// wiring returns every error through.
// Output: ent/entapi_errors.go
//
// It names ent's own IsNotFound and IsConstraintError, which are generated into
// the consumer's package rather than exported by the framework — so this one
// line is the only place where the two halves can meet, and it is why the
// runtime takes predicates as function values instead of importing ent.
func (e *Extension) renderErrorMapFile(g *gen.Graph) (pendingFile, error) {
	tmpl, err := template.New("errors").
		Funcs(e.templateFuncMap()).
		Parse(errorMapTemplate)
	if err != nil {
		return pendingFile{}, fmt.Errorf("failed to parse error classifier template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, g); err != nil {
		return pendingFile{}, fmt.Errorf("failed to render error classifier template: %w", err)
	}

	outputPath := filepath.Join(g.Config.Target, errorMapFileName)

	formatted, err := formatFile(outputPath, buf.Bytes())
	if err != nil {
		return pendingFile{}, err
	}
	return pendingFile{path: outputPath, content: formatted}, nil
}

// renderHTTPFile renders and formats the package-level APIHandler and endpoint
// manifest. Output: ent/entapi_http.go
func (e *Extension) renderHTTPFile(g *gen.Graph) (pendingFile, error) {
	tmpl, err := template.New("http").
		Funcs(e.templateFuncMap()).
		Parse(httpTemplate)
	if err != nil {
		return pendingFile{}, fmt.Errorf("failed to parse HTTP template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, g); err != nil {
		return pendingFile{}, fmt.Errorf("failed to render HTTP template: %w", err)
	}

	outputPath := filepath.Join(g.Config.Target, httpFileName)
	formatted, err := formatFile(outputPath, buf.Bytes())
	if err != nil {
		return pendingFile{}, err
	}
	return pendingFile{path: outputPath, content: formatted}, nil
}

// renderOpenAPIFile renders the OpenAPI 3.1 document describing the generated
// HTTP surface. Output: ent/openapi.yaml
//
// It is the one render*File that does NOT call formatFile, and the omission is
// forced rather than chosen: imports.Process parses its input as Go, so feeding
// it YAML fails, and a formatting failure aborts the whole run in phase 1. The
// honest consequence is stated in the template header — this file alone has no
// syntax gate before it reaches disk, and a template bug here is caught by the
// fixture assertions and the e2e validator instead.
func (e *Extension) renderOpenAPIFile(g *gen.Graph) (pendingFile, error) {
	tmpl, err := template.New("openapi").
		Funcs(e.templateFuncMap()).
		Parse(openapiTemplate)
	if err != nil {
		return pendingFile{}, fmt.Errorf("failed to parse OpenAPI template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, g); err != nil {
		return pendingFile{}, fmt.Errorf("failed to render OpenAPI template: %w", err)
	}

	return pendingFile{path: filepath.Join(g.Config.Target, openapiFileName), content: buf.Bytes()}, nil
}

// renderOpenAPIEmbedFile renders and formats the file that embeds the document
// above and serves it. Output: ent/entapi_openapi.go
func (e *Extension) renderOpenAPIEmbedFile(g *gen.Graph) (pendingFile, error) {
	tmpl, err := template.New("openapi_embed").
		Funcs(e.templateFuncMap()).
		Parse(openapiEmbedTemplate)
	if err != nil {
		return pendingFile{}, fmt.Errorf("failed to parse OpenAPI embed template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, g); err != nil {
		return pendingFile{}, fmt.Errorf("failed to render OpenAPI embed template: %w", err)
	}

	outputPath := filepath.Join(g.Config.Target, openapiEmbedFileName)
	formatted, err := formatFile(outputPath, buf.Bytes())
	if err != nil {
		return pendingFile{}, err
	}
	return pendingFile{path: outputPath, content: formatted}, nil
}

// renderSoftDeleteFile renders and formats the soft-delete traverser and the
// delete-rewriting hook for the whole graph at once.
// Output: ent/entapi_softdelete.go
//
// It returns the zero pendingFile — an empty path — when no entity embeds
// entapi.SoftDeleteMixin, in which case nothing is written and
// removeStaleArtifacts deletes any file an earlier run left. A file holding an
// empty type switch would compile, but it would also leave unexplained dead
// helpers in the consumer's package.
func (e *Extension) renderSoftDeleteFile(g *gen.Graph) (pendingFile, error) {
	if len(softDeleteTypes(g)) == 0 {
		return pendingFile{}, nil
	}

	tmpl, err := template.New("softdelete").
		Funcs(e.templateFuncMap()).
		Parse(softDeleteTemplate)
	if err != nil {
		return pendingFile{}, fmt.Errorf("failed to parse soft-delete template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, g); err != nil {
		return pendingFile{}, fmt.Errorf("failed to render soft-delete template: %w", err)
	}

	outputPath := filepath.Join(g.Config.Target, softDeleteFileName)

	formatted, err := formatFile(outputPath, buf.Bytes())
	if err != nil {
		return pendingFile{}, err
	}
	return pendingFile{path: outputPath, content: formatted}, nil
}

// formatFile runs the generated Go source through goimports. It is a pure
// function of the bytes — it touches no disk — which is what lets phase 1 of
// generatePerTypeFiles catch a template bug before any file is replaced.
//
// A formatting failure aborts. imports.Process only fails on source it cannot
// parse, which for a generator means a template emitted invalid Go — a bug in
// this package, not a formatting blemish. Writing it anyway turns that bug into
// an unexplained compile error in the consumer's repository, with a successful
// exit code on the generation run.
//
// path is passed for two reasons: imports.Process resolves imports relative to
// it, and it is what names the offending file in the error.
func formatFile(path string, raw []byte) ([]byte, error) {
	formatted, err := imports.Process(path, raw, nil)
	if err != nil {
		return nil, fmt.Errorf("goimports failed for %s (generated source is not valid Go): %w", path, err)
	}
	return formatted, nil
}

// writeFormatted writes already-formatted source to disk, atomically for that
// one file: the content goes to a temporary file in the target directory and is
// renamed into place, so no reader ever observes a partial file and a failure
// here leaves the previous content of THIS path intact.
//
// Run-level atomicity is a separate property and belongs to its caller; see
// generatePerTypeFiles.
func writeFormatted(path string, formatted []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp")
	if err != nil {
		return fmt.Errorf("failed to create temporary file for %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer func() {
		// No-op once the rename succeeded; removes the temporary file on every
		// path that did not get that far.
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(formatted); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write file %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to write file %s: %w", path, err)
	}
	// CreateTemp uses 0600; generated sources are world-readable like any other
	// checked-in file.
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return fmt.Errorf("failed to set permissions on %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("failed to write file %s: %w", path, err)
	}
	return nil
}

// templateFuncMap returns the combined template function map with Ent standard functions.
//
// The layering is Ent's gen.Funcs first, then templateFuncs(), then the
// configuration closures — so a later source wins on a name collision, silently
// and invisibly at the call site. That shadowing hazard is neutralised at the
// source rather than managed here: templateFuncs() is required to stay
// disjoint from gen.Funcs, which TestTemplateFuncsDoNotShadowEntBuiltins
// enforces. Anything Ent already supplies (lower, hasPrefix, camel, snake, …)
// reaches the templates from gen.Funcs untouched.
//
// Keep the order as-is. If a genuine override is ever needed, it belongs here
// as an explicit, named exception with a comment — not as a quiet same-named
// entry in templateFuncs().
func (e *Extension) templateFuncMap() template.FuncMap {
	funcs := make(template.FuncMap, len(gen.Funcs))
	for k, v := range gen.Funcs {
		funcs[k] = v
	}

	for k, v := range templateFuncs() {
		funcs[k] = v
	}

	pkg := e.Config.EntAPIPackage
	funcs["entapiPkg"] = func() string { return pkg }
	strictQueryOperators := e.Config.StrictQueryOperators
	// Configuration closures deliberately stay out of templateFuncs().
	funcs["strictQueryOperators"] = func() bool { return strictQueryOperators }

	// The two OpenAPI info fields are closures for the same reason entapiPkg
	// is: they are configuration, not a property of the graph. openapiTitle
	// still takes the graph, because its DEFAULT is one — the ent package name
	// — and computing that in the template would put the policy there.
	title, version := e.Config.OpenAPITitle, e.Config.OpenAPIVersion
	funcs["openapiTitle"] = func(g *gen.Graph) string {
		if title != "" {
			return title
		}
		if g == nil || g.Config == nil {
			return "API"
		}
		return path.Base(g.Config.Package) + " API"
	}
	funcs["openapiVersion"] = func() string {
		if version != "" {
			return version
		}
		return defaultOpenAPIVersion
	}

	return funcs
}

// Option is a function type for configuring the extension.
type Option func(*ExtensionConfig)

// WithBaseService and WithBaseHandler have been removed along with the
// templates they selected (#29). A call site that passed them now omits them;
// see the migration note in README.md.

// WithEntAPIPackage sets the import path for the entapi package
func WithEntAPIPackage(pkg string) Option {
	return func(c *ExtensionConfig) {
		c.EntAPIPackage = pkg
	}
}

// WithStrictQueryOperators rejects unrecognised query operator prefixes.
func WithStrictQueryOperators() Option {
	return func(c *ExtensionConfig) {
		c.StrictQueryOperators = true
	}
}

// WithOpenAPITitle sets info.title of the generated openapi.yaml.
func WithOpenAPITitle(title string) Option {
	return func(c *ExtensionConfig) {
		c.OpenAPITitle = title
	}
}

// WithOpenAPIVersion sets info.version of the generated openapi.yaml.
func WithOpenAPIVersion(version string) Option {
	return func(c *ExtensionConfig) {
		c.OpenAPIVersion = version
	}
}

// NewExtensionWithOptions creates a new extension using functional options.
func NewExtensionWithOptions(opts ...Option) *Extension {
	config := &ExtensionConfig{}

	for _, opt := range opts {
		opt(config)
	}

	return NewExtension(config)
}

// Annotations returns global annotations for the extension
func (e *Extension) Annotations() []entc.Annotation {
	return []entc.Annotation{
		&ConfigAnnotation{Config: e.Config},
	}
}

// Options returns the extension options
func (e *Extension) Options() []entc.Option {
	return []entc.Option{}
}

// ConfigAnnotation implements entc.Annotation for extension configuration
type ConfigAnnotation struct {
	Config *ExtensionConfig
}

// Name returns the annotation name.
func (ConfigAnnotation) Name() string {
	return "ExtensionConfig"
}
