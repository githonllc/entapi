package entdomain

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
	"golang.org/x/tools/imports"
)

// Extension is the entdomain Ent extension.
type Extension struct {
	// Config holds the extension configuration.
	Config *ExtensionConfig
}

// ExtensionConfig holds configuration for the extension.
//
// GenerateBaseService and GenerateBaseHandler used to live here. Both are gone
// with the templates they selected (#29); every generated artifact is now
// emitted unconditionally for an annotated entity, so there is nothing left to
// switch on. See the migration note in README.md.
type ExtensionConfig struct {
	// EntDomainPackage is the import path generated code imports for the Layer 2
	// runtime — ErrValidation, ListRequest, ListPage, the error mapper.
	// Default: "github.com/githonllc/entdomain/runtime"
	EntDomainPackage string
}

// defaultEntDomainPackage is the RUNTIME package, not this one (#15).
//
// The two were the same string until the runtime types moved out, and that is
// what put the embedded templates and the template loader's init into every
// consumer's production binary: generated code imported a sentinel error and
// linked the generator. They are now different packages, and
// TestRuntimePackageIsGeneratorFree is what keeps them apart.
//
// The runtime package is still named `entdomain`, so this changes the import
// LINE in generated output and nothing else — every `entdomain.X` reference in
// every generated file and every consumer call site is unaffected.
const defaultEntDomainPackage = "github.com/githonllc/entdomain/runtime"

// NewExtension creates a new extension instance
func NewExtension(config *ExtensionConfig) *Extension {
	if config == nil {
		config = &ExtensionConfig{}
	}
	if config.EntDomainPackage == "" {
		config.EntDomainPackage = defaultEntDomainPackage
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

// Templates returns an empty template list — the old GraphTemplate approach is no longer used.
func (e *Extension) Templates() []*gen.Template {
	return []*gen.Template{} // removed legacy GraphTemplate generation
}

// perTypeFileName is the file one per-type template's output for node lands in:
// ent/{entity}_dto.go, _filter.go and _wiring.go.
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
		// Separate files are produced for each Type that has entdomain
		// annotations. Entities without annotations are skipped to avoid empty
		// generated files.
		//
		// written records this run's output set, which is what tells a file
		// left over from an earlier run apart from one just produced.
		var pending []pendingFile
		written := make(map[string]bool)
		wiredAny := false
		for _, node := range g.Nodes {
			if len(domainFields(node)) == 0 {
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
		}

		// The soft-delete traverser is generated once per GRAPH, not per type:
		// it is one type switch over the entities embedding
		// entdomain.SoftDeleteMixin. It sits outside the loop above for a
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

// renderErrorMapFile renders and formats the package-level error classifier the
// wiring returns every error through.
// Output: ent/entdomain_errors.go
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

// renderSoftDeleteFile renders and formats the soft-delete traverser, the
// delete-rewriting hook and the single registration function, for the whole
// graph at once.
// Output: ent/entdomain_softdelete.go
//
// It returns the zero pendingFile — an empty path — when no entity embeds
// entdomain.SoftDeleteMixin, in which case nothing is written and
// removeStaleArtifacts deletes any file an earlier run left. A file holding an
// empty type switch would compile, but it would also publish a
// RegisterSoftDelete that quietly does nothing.
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
// entdomainPkg closure — so a later source wins on a name collision, silently
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

	pkg := e.Config.EntDomainPackage
	funcs["entdomainPkg"] = func() string { return pkg }

	return funcs
}

// Option is a function type for configuring the extension.
type Option func(*ExtensionConfig)

// WithBaseService and WithBaseHandler have been removed along with the
// templates they selected (#29). They were the only two options, so a call site
// that passed them now passes nothing — which is also the whole configuration
// this extension has left, apart from the import path below. See the migration
// note in README.md.

// WithEntDomainPackage sets the import path for the entdomain package
func WithEntDomainPackage(pkg string) Option {
	return func(c *ExtensionConfig) {
		c.EntDomainPackage = pkg
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
