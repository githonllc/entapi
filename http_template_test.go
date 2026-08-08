package entapi

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
	"testing"
)

func TestHandlerTemplateReadsMiddleFunctionThroughReceiver(t *testing.T) {
	root := repoRoot(t)
	g := loadFixtureGraph(t, fixtureSchemaDir(root, "reservednames"), fixtureEntPkgPath("reservednames"))
	probe := nodeNamed(t, g, "Probe")
	ext := NewExtensionWithOptions()

	src := string(renderTemplate(t, ext, "handler", handlerTemplate, probe))
	assertSourceContains(t, src,
		"type CreateProbeFn func(context.Context, *Client, *ValidProbeCreateRequest) (*ProbeResponse, error)",
		"func (f CreateProbeFn) applyOption(h *APIHandler)",
		`panic("entapi: CreateProbeFn is nil")`,
		"h.createProbe = f",
		"response, err := h.createProbe(r.Context(), h.client, validated)",
	)
	if strings.Contains(src, "response, err := CreateProbe(") {
		t.Fatal("create handler calls the default wiring function directly instead of reading h.createProbe at request time")
	}
}

func TestHTTPTemplateBuildsRouteManifestAndLiteralMountLoop(t *testing.T) {
	root := repoRoot(t)
	g := loadFixtureGraph(t, fixtureSchemaDir(root, "reservednames"), fixtureEntPkgPath("reservednames"))
	ext := NewExtensionWithOptions()

	src := string(renderGraphTemplate(t, ext, "http", httpTemplate, g))
	assertSourceContains(t, src,
		"type APIOption interface{ applyOption(*APIHandler) }",
		"type APIHandler struct {",
		"createProbe CreateProbeFn",
		"createProbe: CreateProbe,",
		`Path:    "/probes"`,
		`Path:    "/probes/{id}"`,
		"Handler: http.HandlerFunc(h.handleCreateProbe)",
		"func API(client *Client) *APIHandler",
		"func (h *APIHandler) With(opts ...APIOption) *APIHandler",
		`panic(fmt.Sprintf("entapi: APIOption at index %d is nil", i))`,
		"opt.applyOption(h)",
		"func (h *APIHandler) Routes() []entapi.Route",
		"return append([]entapi.Route(nil), h.routes...)",
		"func (h *APIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request)",
		"for _, rt := range h.routes {\n\t\tmux.Handle(rt.Method+\" \"+rt.Path, rt.Handler)\n\t}",
	)
}

func TestHTTPTemplateAPIOptionHasOnlyUnexportedMethod(t *testing.T) {
	root := repoRoot(t)
	g := loadFixtureGraph(t, fixtureSchemaDir(root, "reservednames"), fixtureEntPkgPath("reservednames"))
	src := renderGraphTemplate(t, NewExtensionWithOptions(), "http", httpTemplate, g)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "entapi_http.go", src, 0)
	if err != nil {
		t.Fatalf("parse rendered HTTP source: %v\n%s", err, src)
	}
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != "APIOption" {
				continue
			}
			iface, ok := typeSpec.Type.(*ast.InterfaceType)
			if !ok || len(iface.Methods.List) != 1 {
				t.Fatalf("APIOption = %#v, want an interface with one method", typeSpec.Type)
			}
			method := iface.Methods.List[0]
			if len(method.Names) != 1 || method.Names[0].Name != "applyOption" || method.Names[0].IsExported() {
				t.Fatalf("APIOption method = %#v, want only unexported applyOption", method.Names)
			}
			return
		}
	}
	t.Fatal("APIOption declaration not found")
}

func TestHandlerMiddleValidationStatusDependsOnOperation(t *testing.T) {
	root := repoRoot(t)
	g := loadFixtureGraph(t, fixtureSchemaDir(root, "httpdemo"), fixtureEntPkgPath("httpdemo"))
	article := nodeNamed(t, g, "Article")
	src := renderTemplate(t, NewExtensionWithOptions(), "handler", handlerTemplate, article)

	bodies := map[string]string{}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "article_handler.go", src, 0)
	if err != nil {
		t.Fatalf("parse rendered handler: %v\n%s", err, src)
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil {
			continue
		}
		var body strings.Builder
		if err := printer.Fprint(&body, fset, fn.Body); err != nil {
			t.Fatalf("print %s: %v", fn.Name.Name, err)
		}
		bodies[fn.Name.Name] = body.String()
	}

	assertValidationStatus := func(name, status string) {
		t.Helper()
		body := bodies[name]
		want := "case entapi.IsValidation(err):\n\t\t\tstatus = " + status
		if !strings.Contains(body, want) {
			t.Errorf("%s validation arm does not map to %s\n%s", name, status, body)
		}
	}
	assertValidationStatus("handleListArticles", "http.StatusBadRequest")
	assertValidationStatus("handleCreateArticle", "http.StatusUnprocessableEntity")
	assertValidationStatus("handlePatchArticle", "http.StatusUnprocessableEntity")
	for _, name := range []string{"handleGetArticle", "handleDeleteArticle"} {
		if strings.Contains(bodies[name], "entapi.IsValidation(") {
			t.Errorf("%s must not classify an impossible middle-step validation error\n%s", name, bodies[name])
		}
	}
}

func assertSourceContains(t *testing.T, src string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(src, want) {
			t.Errorf("generated source does not contain %q\n%s", want, src)
		}
	}
}
