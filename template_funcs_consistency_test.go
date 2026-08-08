package entapi

import (
	"io/fs"
	"path"
	"sort"
	"strings"
	"testing"
	"text/template/parse"

	"entgo.io/ent/entc/gen"
)

// templateBuiltins are the functions text/template supplies on its own.
// They are always callable and never appear in any FuncMap.
var templateBuiltins = map[string]bool{
	"and": true, "call": true, "html": true, "index": true, "slice": true,
	"js": true, "len": true, "not": true, "or": true, "print": true,
	"printf": true, "println": true, "urlquery": true,
	"eq": true, "ge": true, "gt": true, "le": true, "lt": true, "ne": true,
}

// collectIdentifiers walks a parsed template tree and records every function
// invocation it contains. The parser emits an *parse.IdentifierNode only for a
// bare name in function position, so this is exactly the set of names the
// template needs the FuncMap (or the builtins) to supply.
func collectIdentifiers(n parse.Node, out map[string]bool) {
	if n == nil {
		return
	}
	switch node := n.(type) {
	case *parse.ListNode:
		if node == nil {
			return
		}
		for _, child := range node.Nodes {
			collectIdentifiers(child, out)
		}
	case *parse.ActionNode:
		collectIdentifiers(node.Pipe, out)
	case *parse.PipeNode:
		if node == nil {
			return
		}
		for _, cmd := range node.Cmds {
			collectIdentifiers(cmd, out)
		}
	case *parse.CommandNode:
		for _, arg := range node.Args {
			collectIdentifiers(arg, out)
		}
	case *parse.IdentifierNode:
		out[node.Ident] = true
	case *parse.IfNode:
		collectBranch(&node.BranchNode, out)
	case *parse.RangeNode:
		collectBranch(&node.BranchNode, out)
	case *parse.WithNode:
		collectBranch(&node.BranchNode, out)
	case *parse.TemplateNode:
		collectIdentifiers(node.Pipe, out)
	}
}

func collectBranch(b *parse.BranchNode, out map[string]bool) {
	collectIdentifiers(b.Pipe, out)
	collectIdentifiers(b.List, out)
	collectIdentifiers(b.ElseList, out)
}

// templateInvocations parses one embedded template and returns the function
// names it invokes.
func templateInvocations(t *testing.T, name, src string) map[string]bool {
	t.Helper()

	tree := parse.New(name)
	tree.Mode = parse.SkipFuncCheck
	parsed, err := tree.Parse(src, "{{", "}}", map[string]*parse.Tree{})
	if err != nil {
		t.Fatalf("failed to parse %s: %v", name, err)
	}

	idents := make(map[string]bool)
	collectIdentifiers(parsed.Root, idents)
	return idents
}

// TestTemplateInvocationsAreRegistered is the drift detector between
// templates/*.tmpl and the function registry. The templates are the only real
// consumer of templateFuncs(), so the assertion is derived from them rather
// than from a hardcoded name list: every function a template calls must be
// supplied at generation time, or generation panics at template parse time in
// a downstream project.
func TestTemplateInvocationsAreRegistered(t *testing.T) {
	entries, err := fs.ReadDir(templateFS, "templates")
	if err != nil {
		t.Fatalf("failed to read embedded templates: %v", err)
	}

	// The map the generator actually parses templates with: Ent's gen.Funcs,
	// overlaid with templateFuncs(), plus the entapiPkg closure.
	available := NewExtension(nil).templateFuncMap()

	used := make(map[string]bool)
	scanned := 0

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tmpl") {
			continue
		}
		src, err := templateFS.ReadFile(path.Join("templates", entry.Name()))
		if err != nil {
			t.Fatalf("failed to read embedded template %s: %v", entry.Name(), err)
		}
		scanned++

		for name := range templateInvocations(t, entry.Name(), string(src)) {
			used[name] = true
			if templateBuiltins[name] {
				continue
			}
			if _, ok := available[name]; !ok {
				t.Errorf("templates/%s invokes %q, which no template function supplies; register it in templateFuncs() (funcs.go)", entry.Name(), name)
			}
		}
	}

	if scanned == 0 {
		t.Fatal("no embedded templates were scanned; this test would pass vacuously")
	}

	// The reverse direction: a registered entry no template invokes is dead
	// weight that a reader cannot distinguish from live code. Worse, several
	// such entries once emitted code for a repository-shaped API that no longer
	// exists, so reconnecting one would have produced wrong queries rather than
	// a compile error. Registration is therefore the claim "a template calls
	// this", and this assertion is what makes the claim true.
	var unused []string
	for name := range templateFuncs() {
		if !used[name] {
			unused = append(unused, name)
		}
	}
	sort.Strings(unused)
	if len(unused) > 0 {
		t.Errorf("templateFuncs() registers %d entries invoked by no template: %s\ndelete them from templateFuncs() (funcs.go), or add the template call that needs them", len(unused), strings.Join(unused, ", "))
	}
}

// TestTemplateFuncsDoNotShadowEntBuiltins pins the second half of the registry
// contract. templateFuncMap() copies gen.Funcs in first and then overlays
// templateFuncs(), so a same-named entry silently replaces an Ent builtin for
// every template — including any Ent-authored template a consumer renders with
// the same map. The override is invisible at the call site: the template still
// reads {{ lower $.Name }}, only the implementation changes.
//
// Rather than document a hazard, the registry keeps the two namespaces
// disjoint. Anything Ent already supplies is used from Ent.
func TestTemplateFuncsDoNotShadowEntBuiltins(t *testing.T) {
	var shadowed []string
	for name := range templateFuncs() {
		if _, ok := gen.Funcs[name]; ok {
			shadowed = append(shadowed, name)
		}
	}
	sort.Strings(shadowed)
	if len(shadowed) > 0 {
		t.Errorf("templateFuncs() shadows %d Ent builtin(s): %s\nremove the entry and let gen.Funcs supply it, or rename this package's function", len(shadowed), strings.Join(shadowed, ", "))
	}
}
