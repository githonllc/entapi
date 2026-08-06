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

// ExtensionConfig holds configuration for the extension
type ExtensionConfig struct {
	// GenerateBaseService controls whether BaseService structs are generated
	GenerateBaseService bool

	// GenerateBaseHandler controls whether BaseHandler structs are generated
	GenerateBaseHandler bool

	// EntDomainPackage is the import path for the entdomain package
	// Default: "github.com/githonllc/entdomain"
	EntDomainPackage string
}

const defaultEntDomainPackage = "github.com/githonllc/entdomain"

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

// generatePerTypeFiles is the core Hook that generates separate files for each Type.
func (e *Extension) generatePerTypeFiles(next gen.Generator) gen.Generator {
	return gen.GenerateFunc(func(g *gen.Graph) error {
		// Reject annotations that contradict the ent schema before anything is
		// written — including by ent's own generator, which runs below. A
		// contradiction cannot be generated into code that compiles, so the
		// only honest outcomes are a clear error here or a compile error in the
		// consumer's package. See schema_conflicts.go.
		if err := checkGraphConflicts(g, e.Config); err != nil {
			return err
		}

		// Run the standard generation first
		if err := next.Generate(g); err != nil {
			return err
		}

		// Generate separate files for each Type that has entdomain annotations.
		// Entities without annotations are skipped to avoid empty generated files.
		//
		// written records this run's output set, which is what tells a file
		// left over from an earlier run apart from one just produced.
		written := make(map[string]bool)
		for _, node := range g.Nodes {
			if len(domainFields(node)) == 0 {
				continue
			}

			// Generate DTO file → ent/{entity}_dto.go
			path, err := e.generateDTOFile(g, node)
			if err != nil {
				return fmt.Errorf("failed to generate %s DTO: %w", node.Name, err)
			}
			written[path] = true

			// Generate the query surface → ent/{entity}_filter.go
			path, err = e.generateFilterFile(g, node)
			if err != nil {
				return fmt.Errorf("failed to generate %s filter: %w", node.Name, err)
			}
			written[path] = true

			// Generate base service file → ent/{entity}_base_service.go
			if e.Config.GenerateBaseService {
				path, err := e.generateBaseServiceFile(g, node)
				if err != nil {
					return fmt.Errorf("failed to generate %s base service file: %w", node.Name, err)
				}
				written[path] = true
			}

			// Generate base handler file → ent/{entity}_base_handler.go
			if e.Config.GenerateBaseHandler {
				path, err := e.generateBaseHandlerFile(g, node)
				if err != nil {
					return fmt.Errorf("failed to generate %s base handler file: %w", node.Name, err)
				}
				written[path] = true
			}
		}

		// Only once every file is on disk: a run that failed partway must not
		// delete anything, or a template bug would take the previous output
		// with it.
		if err := removeStaleArtifacts(g.Config.Target, g.Nodes, written); err != nil {
			return err
		}

		return nil
	})
}

// generateDTOFile generates a DTO file for a single Type.
// Output: ent/{entity}_dto.go
func (e *Extension) generateDTOFile(g *gen.Graph, node *gen.Type) (string, error) {
	tmpl, err := template.New("dto").
		Funcs(e.templateFuncMap()).
		Parse(dtoTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse DTO template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, node); err != nil {
		return "", fmt.Errorf("failed to render DTO template: %w", err)
	}

	filename := fmt.Sprintf("%s_dto.go", strings.ToLower(node.Name))
	outputPath := filepath.Join(g.Config.Target, filename)

	if err := writeFile(outputPath, buf.Bytes()); err != nil {
		return "", err
	}
	return outputPath, nil
}

// generateFilterFile generates the query surface for a single Type: the filter
// struct, its predicates and the sort allow-list.
// Output: ent/{entity}_filter.go
//
// It is emitted for every annotated entity, including one that marks no field
// filterable, searchable or sortable. Such an entity gets an empty filter type
// and an empty allow-list, which is the honest answer — and the safe one, since
// an empty allow-list makes the entity orderable by nothing at all.
func (e *Extension) generateFilterFile(g *gen.Graph, node *gen.Type) (string, error) {
	tmpl, err := template.New("filter").
		Funcs(e.templateFuncMap()).
		Parse(filterTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse filter template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, node); err != nil {
		return "", fmt.Errorf("failed to render filter template: %w", err)
	}

	filename := fmt.Sprintf("%s_filter.go", strings.ToLower(node.Name))
	outputPath := filepath.Join(g.Config.Target, filename)

	if err := writeFile(outputPath, buf.Bytes()); err != nil {
		return "", err
	}
	return outputPath, nil
}

// generateBaseServiceFile generates a base service file for a single Type.
// Output: ent/{entity}_base_service.go
func (e *Extension) generateBaseServiceFile(g *gen.Graph, node *gen.Type) (string, error) {
	tmpl, err := template.New("base_service").
		Funcs(e.templateFuncMap()).
		Parse(baseServiceTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse base service template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, node); err != nil {
		return "", fmt.Errorf("failed to render base service template: %w", err)
	}

	filename := fmt.Sprintf("%s_base_service.go", strings.ToLower(node.Name))
	outputPath := filepath.Join(g.Config.Target, filename)

	if err := writeFile(outputPath, buf.Bytes()); err != nil {
		return "", err
	}
	return outputPath, nil
}

// generateBaseHandlerFile generates a base handler file for a single Type.
// Output: ent/{entity}_base_handler.go
func (e *Extension) generateBaseHandlerFile(g *gen.Graph, node *gen.Type) (string, error) {
	tmpl, err := template.New("base_handler").
		Funcs(e.templateFuncMap()).
		Parse(baseHandlerTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse base handler template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, node); err != nil {
		return "", fmt.Errorf("failed to render base handler template: %w", err)
	}

	filename := fmt.Sprintf("%s_base_handler.go", strings.ToLower(node.Name))
	outputPath := filepath.Join(g.Config.Target, filename)

	if err := writeFile(outputPath, buf.Bytes()); err != nil {
		return "", err
	}
	return outputPath, nil
}

// writeFile formats the generated Go source with goimports and writes it to disk.
//
// A formatting failure aborts. imports.Process only fails on source it cannot
// parse, which for a generator means a template emitted invalid Go — a bug in
// this package, not a formatting blemish. Writing it anyway turns that bug into
// an unexplained compile error in the consumer's repository, with a successful
// exit code on the generation run.
//
// The write is atomic: the content goes to a temporary file in the target
// directory and is renamed into place only after formatting succeeded. A run
// that fails partway therefore leaves the previous run's output untouched
// rather than a half-written file.
func writeFile(path string, content []byte) error {
	formatted, err := imports.Process(path, content, nil)
	if err != nil {
		return fmt.Errorf("goimports failed for %s (generated source is not valid Go): %w", path, err)
	}

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

// WithBaseService controls whether BaseService structs are generated
func WithBaseService(generate bool) Option {
	return func(c *ExtensionConfig) {
		c.GenerateBaseService = generate
	}
}

// WithBaseHandler controls whether BaseHandler structs are generated
func WithBaseHandler(generate bool) Option {
	return func(c *ExtensionConfig) {
		c.GenerateBaseHandler = generate
	}
}

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
