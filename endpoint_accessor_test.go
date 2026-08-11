package entapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"entgo.io/ent/entc/gen"
)

// TestEndpointAccessorsCoverTheManifestExactly is the guard that keeps the named
// accessors and the endpoint manifest from drifting apart (#119).
//
// The two are one surface answering two questions — take this one by name, or
// loop over all of them — and a consumer who mixes them has to be able to assume
// they describe the same endpoints. So the accessor list is not read from the
// template's text, it is DERIVED from resourceOps, the same record the manifest
// is generated from, and compared in both directions:
//
//	forward  every operation resourceOps yields has exactly one accessor, whose
//	         body carries the same Method, Path, Handler, Entity and Op the
//	         manifest entry must carry;
//	reverse  every accessor the template emits belongs to such an operation, plus
//	         OpenAPIEndpoint for the manifest's one Op-less entry.
//
// The third assertion is what makes the value equality structural rather than
// asserted: the h.endpoints literal must consist of nothing but calls to those
// accessors, in that order. A manifest that spelled its own struct literals
// would be the second table the issue exists to refuse, and it would pass the
// first two assertions while being free to disagree with them.
func TestEndpointAccessorsCoverTheManifestExactly(t *testing.T) {
	root := repoRoot(t)
	// httpdemo rather than basic: AuditLog excepts OpDelete, so an accessor set
	// that ignored Except would differ from the derived one here and nowhere in
	// a fixture whose entities expose the full five.
	g := loadFixtureGraph(t, fixtureSchemaDir(root, "httpdemo"), fixtureEntPkgPath("httpdemo"))
	src := renderGraphTemplate(t, NewExtensionWithOptions(), "http", httpTemplate, g)

	accessors := endpointAccessors(t, src)
	want := expectedEndpointAccessors(g)

	for _, name := range want {
		if _, ok := accessors[name]; !ok {
			t.Errorf("no %s method: the operation is in the manifest, so it must also be reachable by name", name)
		}
	}
	for _, name := range sortedKeys(accessors) {
		if !containsString(want, name) {
			t.Errorf("%s is generated but belongs to no operation resourceOps yields and is not OpenAPIEndpoint; "+
				"an accessor with no manifest entry is a route a consumer can register that ServeHTTP does not serve", name)
		}
	}

	// Forward, per field: the accessor body is the manifest entry.
	for _, node := range g.Nodes {
		path := routePath(node)
		for _, op := range resourceOps(node) {
			name := op.Function + "Endpoint"
			assertEndpointLiteral(t, accessors, name, map[string]string{
				"Method":  strconv.Quote(op.Method),
				"Path":    strconv.Quote(path + op.PathSuffix),
				"Handler": "http.HandlerFunc(h." + op.Handler + ")",
				"Entity":  strconv.Quote(node.Name),
				"Op":      strconv.Quote(string(op.Name)),
			})
		}
	}
	assertEndpointLiteral(t, accessors, "OpenAPIEndpoint", map[string]string{
		"Method":  strconv.Quote("GET"),
		"Path":    strconv.Quote("/openapi.yaml"),
		"Handler": "http.HandlerFunc(serveOpenAPI)",
	})

	// Structural: the manifest is those accessors, not a copy of them.
	got := endpointManifestCalls(t, src)
	if len(got) != len(want) {
		t.Fatalf("h.endpoints has %d elements %v, want %d accessor calls %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("h.endpoints element %d is h.%s(), want h.%s(); the manifest must be built from the accessors in manifest order",
				i, got[i], want[i])
		}
	}
}

// TestExceptedOperationGeneratesNoEndpointAccessor pins the compile-time half of
// the issue: Except must remove the method, not merely the manifest row. An
// accessor that survived would make ent.API(client).CreateAccountEndpoint()
// compile and hand back an endpoint no ServeHTTP serves.
func TestExceptedOperationGeneratesNoEndpointAccessor(t *testing.T) {
	root := repoRoot(t)
	g := loadFixtureGraph(t, fixtureSchemaDir(root, "createexcepted"), fixtureEntPkgPath("createexcepted"))
	src := renderGraphTemplate(t, NewExtensionWithOptions(), "http", httpTemplate, g)

	accessors := endpointAccessors(t, src)
	if _, ok := accessors["CreateAccountEndpoint"]; ok {
		t.Error("CreateAccountEndpoint is generated despite Except(api.OpCreate)")
	}
	for _, name := range []string{
		"ListAccountsEndpoint", "GetAccountEndpoint", "PatchAccountEndpoint",
		"DeleteAccountEndpoint", "OpenAPIEndpoint",
	} {
		if _, ok := accessors[name]; !ok {
			t.Errorf("%s is missing; only the excepted operation may lose its accessor", name)
		}
	}

	// And the same thing on the committed output, so the absence is asserted
	// against the bytes a consumer of that fixture actually compiles.
	file := filepath.Join(root, "internal", "fixtures", "createexcepted", "createexceptedent", "entapi_http.go")
	generated, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	if strings.Contains(string(generated), "CreateAccountEndpoint") {
		t.Errorf("%s mentions CreateAccountEndpoint despite Except(api.OpCreate)", file)
	}
	if !strings.Contains(string(generated), "func (h *APIHandler) GetAccountEndpoint() entapi.Endpoint") {
		t.Errorf("%s does not declare GetAccountEndpoint", file)
	}
}

// expectedEndpointAccessors is the accessor list derived from resourceOps, in
// manifest order, with the document's accessor last where the manifest carries
// its entry.
func expectedEndpointAccessors(g *gen.Graph) []string {
	var out []string
	for _, node := range g.Nodes {
		for _, op := range resourceOps(node) {
			out = append(out, op.Function+"Endpoint")
		}
	}
	return append(out, "OpenAPIEndpoint")
}

// endpointAccessors returns every zero-argument *APIHandler method returning an
// entapi.Endpoint, keyed by name.
//
// The match is the SIGNATURE alone — receiver, no parameters, exactly one
// entapi.Endpoint result — and deliberately not the name. A name filter would
// make the set agree with the template's naming convention by construction: an
// accessor the template emitted under some other spelling would be skipped
// rather than reported, and the reverse direction above would pass on a surface
// that has no manifest entry. That is the same signature rule the e2e half
// applies through reflect, so the two tests select the same set for the same
// reason.
//
// A method whose name ends in …Endpoint but whose signature does not match is
// the one shape worth reporting rather than skipping: it is either an accessor
// that grew an argument — an endpoint accessor is a name, not a lookup — or a
// near-miss that a reader would take for one.
func endpointAccessors(t *testing.T, src []byte) map[string]*ast.FuncDecl {
	t.Helper()

	out := map[string]*ast.FuncDecl{}
	for _, decl := range parseGeneratedHTTP(t, src).Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
			continue
		}
		star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		if ident, ok := star.X.(*ast.Ident); !ok || ident.Name != "APIHandler" {
			continue
		}
		takesNothing := fn.Type.Params.NumFields() == 0
		returnsEndpoint := fn.Type.Results.NumFields() == 1 &&
			types.ExprString(fn.Type.Results.List[0].Type) == "entapi.Endpoint"
		if !takesNothing || !returnsEndpoint {
			if strings.HasSuffix(fn.Name.Name, "Endpoint") {
				t.Errorf("%s is named like an endpoint accessor but does not take nothing and return exactly one entapi.Endpoint",
					fn.Name.Name)
			}
			continue
		}
		out[fn.Name.Name] = fn
	}
	return out
}

// assertEndpointLiteral checks the entapi.Endpoint the named accessor returns,
// field by field, against expressions derived from resourceOps. Fields absent
// from want must be absent from the literal, which is what pins OpenAPIEndpoint
// as the manifest's Entity-less, Op-less entry.
//
// "Returns" is meant literally: the accessor must be one return of one
// entapi.Endpoint composite literal, which is what templates/http.tmpl emits.
// That requirement is the check, not a convenience — see the comment on it
// below.
func assertEndpointLiteral(t *testing.T, accessors map[string]*ast.FuncDecl, name string, want map[string]string) {
	t.Helper()

	fn, ok := accessors[name]
	if !ok {
		return // already reported by the caller's coverage check
	}
	// The literal has to be the one the accessor RETURNS, not merely one that
	// appears somewhere in its body: a body that built a correct literal, bound
	// it to a name, mutated a field and returned the variable would satisfy any
	// walk that keeps the last match while returning something else entirely.
	// So the shape is pinned instead — the template emits exactly one return of
	// one composite literal, and anything else is reported rather than searched.
	if fn.Body == nil {
		t.Errorf("%s has no body, so it returns no endpoint to check", name)
		return
	}
	if len(fn.Body.List) != 1 {
		t.Errorf("%s is no longer a single literal return; its body has %d statements, so what it returns "+
			"is not what this test can check field by field", name, len(fn.Body.List))
		return
	}
	ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		t.Errorf("%s is no longer a single literal return; its only statement must be a return of one value", name)
		return
	}
	lit, ok := ret.Results[0].(*ast.CompositeLit)
	if !ok || types.ExprString(lit.Type) != "entapi.Endpoint" {
		t.Errorf("%s does not return an entapi.Endpoint composite literal directly; it returns %s",
			name, types.ExprString(ret.Results[0]))
		return
	}

	got := map[string]string{}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			t.Errorf("%s builds its endpoint positionally; a field added to entapi.Endpoint would silently shift", name)
			return
		}
		got[types.ExprString(kv.Key)] = types.ExprString(kv.Value)
	}
	for _, field := range []string{"Method", "Path", "Handler", "Entity", "Op"} {
		if got[field] != want[field] {
			t.Errorf("%s sets %s = %q, want %q", name, field, got[field], want[field])
		}
	}
}

// endpointManifestCalls returns the accessor names the API constructor assigns
// to h.endpoints, in order, failing on any element that is not such a call.
func endpointManifestCalls(t *testing.T, src []byte) []string {
	t.Helper()

	var api *ast.FuncDecl
	for _, decl := range parseGeneratedHTTP(t, src).Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == "API" {
			api = fn
		}
	}
	if api == nil {
		t.Fatal("no API constructor in the rendered HTTP source")
	}

	var lit *ast.CompositeLit
	ast.Inspect(api.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		if types.ExprString(assign.Lhs[0]) != "h.endpoints" {
			return true
		}
		if cl, ok := assign.Rhs[0].(*ast.CompositeLit); ok {
			lit = cl
		}
		return true
	})
	if lit == nil {
		t.Fatal("API does not assign a composite literal to h.endpoints")
	}

	out := make([]string, 0, len(lit.Elts))
	for i, elt := range lit.Elts {
		call, ok := elt.(*ast.CallExpr)
		if !ok || len(call.Args) != 0 {
			t.Fatalf("h.endpoints element %d is %s, not a call to an accessor; the manifest must share the accessors' construction rather than carry a second table",
				i, types.ExprString(elt))
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || types.ExprString(sel.X) != "h" {
			t.Fatalf("h.endpoints element %d calls %s, want a method on h", i, types.ExprString(call.Fun))
		}
		out = append(out, sel.Sel.Name)
	}
	return out
}

func parseGeneratedHTTP(t *testing.T, src []byte) *ast.File {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "entapi_http.go", src, 0)
	if err != nil {
		t.Fatalf("rendered HTTP source is not valid Go: %v\n%s", err, src)
	}
	return file
}
