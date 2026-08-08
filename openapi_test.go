package entapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// generateOpenAPIDocument runs a real generation of one fixture schema into a
// temporary directory and returns the OpenAPI document it wrote.
//
// It deliberately does NOT read the committed copy under
// internal/fixtures/<dir>/<dir>ent: TestCodegenFixtures writes that file, and
// nothing orders the two tests within this package. Generating here makes the
// assertion independent of test order and of whatever the tree happened to
// carry.
func generateOpenAPIDocument(t *testing.T, fixtureDir string, opts ...Option) string {
	t.Helper()

	root := repoRoot(t)
	target := t.TempDir()
	if err := generateFixture(fixtureDir, fixtureSchemaDir(root, fixtureDir), target, opts, nil); err != nil {
		t.Fatalf("fixture %q: generation failed: %v", fixtureDir, err)
	}
	content, err := os.ReadFile(filepath.Join(target, "openapi.yaml"))
	if err != nil {
		t.Fatalf("fixture %q: no OpenAPI document was generated: %v", fixtureDir, err)
	}
	return string(content)
}

// TestOpenAPIDocumentHeaderAndDefaults pins the four decisions that cannot be
// read back out of the document by an OpenAPI parser (#76):
//
//   - the first line is the ownership marker, in comment form, so cleanup owns
//     the file and deleting that one line hands it to the consumer permanently;
//   - the version is 3.1.0, not 3.0.x;
//   - no `servers` entry is emitted, because a mount prefix is a deployment
//     fact and a guessed one can only lie;
//   - info.title and info.version fall back to the ent package name plus "API"
//     and to 0.0.0, neither of which is read from working-tree state.
func TestOpenAPIDocumentHeaderAndDefaults(t *testing.T) {
	doc := generateOpenAPIDocument(t, "basic")

	firstLine, _, _ := strings.Cut(doc, "\n")
	if !strings.HasPrefix(firstLine, "#") || !strings.Contains(firstLine, generatedMarker) {
		t.Errorf("first line = %q, want a comment carrying %q", firstLine, generatedMarker)
	}
	if !strings.Contains(doc, "openapi: 3.1.0") {
		t.Error("the document does not declare openapi: 3.1.0")
	}
	for _, line := range strings.Split(doc, "\n") {
		if strings.HasPrefix(line, "servers:") {
			t.Error("the document emits a servers entry; the mount prefix is a deployment fact and a guess can only lie")
		}
	}
	if !strings.Contains(doc, `title: "basicent API"`) {
		t.Errorf("default info.title is not the ent package name plus API:\n%s", doc)
	}
	if !strings.Contains(doc, `version: "0.0.0"`) {
		t.Errorf("default info.version is not 0.0.0:\n%s", doc)
	}
	if !strings.Contains(doc, `  "/widgets":`) {
		t.Errorf("collection path carries a prefix or is missing:\n%s", doc)
	}
}

// TestOpenAPIInfoComesFromExtensionConfiguration pins the seam: title and
// version are generation-time configuration on the extension, not something
// derived from the working tree.
func TestOpenAPIInfoComesFromExtensionConfiguration(t *testing.T) {
	doc := generateOpenAPIDocument(t, "basic",
		WithOpenAPITitle("Widget Service"),
		WithOpenAPIVersion("2.4.1"))

	if !strings.Contains(doc, `title: "Widget Service"`) {
		t.Errorf("configured info.title did not reach the document:\n%s", doc)
	}
	if !strings.Contains(doc, `version: "2.4.1"`) {
		t.Errorf("configured info.version did not reach the document:\n%s", doc)
	}
}

// TestOpenAPIDocumentOmitsExceptedOperations is the derivation criterion: the
// document's operations are resourceOps, the same source templates/http.tmpl
// routes from, so an Excepted operation cannot survive in one and not the
// other. httpdemo's AuditLog excepts OpDelete and its Article excepts nothing.
func TestOpenAPIDocumentOmitsExceptedOperations(t *testing.T) {
	doc := generateOpenAPIDocument(t, "httpdemo")

	articleItem := pathItem(t, doc, "/articles/{id}")
	if !strings.Contains(articleItem, "delete:") {
		t.Errorf("/articles/{id} has no delete operation, but Article excepts nothing:\n%s", articleItem)
	}
	auditItem := pathItem(t, doc, "/audit_logs/{id}")
	if strings.Contains(auditItem, "delete:") {
		t.Errorf("/audit_logs/{id} documents a delete operation despite Except(api.OpDelete):\n%s", auditItem)
	}
}

// TestOpenAPIDocumentDescribesTheQuerySurface pins the criterion the op-in-value
// wire format costs: a filter parameter degrades to a string, so the pattern and
// the description are the only place the contract survives.
func TestOpenAPIDocumentDescribesTheQuerySurface(t *testing.T) {
	doc := generateOpenAPIDocument(t, "httpdemo")

	for _, want := range []string{
		`name: "title"`,
		`"pattern": `,
		`name: "_page"`,
		`name: "_size"`,
		`name: "_sort"`,
		`name: "_q"`,
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("the list operation does not document %s:\n%s", want, doc)
		}
	}
	// The repeated-parameter meaning has no OpenAPI expression that generators
	// honour, so it has to be in the prose or it is nowhere.
	if !strings.Contains(doc, "Repeating the parameter is legal and ANDs the resulting predicates") {
		t.Error("no filter parameter description explains that repeating the parameter ANDs the predicates")
	}
}

// TestOpenAPIDocumentHidesSensitiveFieldsFromResponses is the narrow half of
// #69's rule, restated where it is now observable: a Sensitive field is absent
// from every response and summary schema, and present in the request schemas,
// because silence about a request field would be a different (and wrong) claim.
func TestOpenAPIDocumentHidesSensitiveFieldsFromResponses(t *testing.T) {
	doc := generateOpenAPIDocument(t, "httpdemo")

	response := componentSchema(t, doc, "ArticleResponse")
	if strings.Contains(response, "internal_note") {
		t.Errorf("ArticleResponse documents the Sensitive field:\n%s", response)
	}
	summary := componentSchema(t, doc, "ArticleSummary")
	if strings.Contains(summary, "internal_note") {
		t.Errorf("ArticleSummary documents the Sensitive field:\n%s", summary)
	}
	create := componentSchema(t, doc, "ArticleCreateRequest")
	if !strings.Contains(create, "internal_note") {
		t.Errorf("ArticleCreateRequest omits the Sensitive field, which is settable:\n%s", create)
	}
}

// pathItem returns the YAML block under one path key, so an assertion about one
// path cannot accidentally be satisfied by another.
func pathItem(t *testing.T, doc, path string) string {
	t.Helper()
	return yamlBlock(t, doc, "  "+quoteYAMLForTest(path)+":")
}

// componentSchema returns the YAML block under one components/schemas entry.
func componentSchema(t *testing.T, doc, name string) string {
	t.Helper()
	return yamlBlock(t, doc, "    "+quoteYAMLForTest(name)+":")
}

func quoteYAMLForTest(s string) string { return `"` + s + `"` }

// yamlBlock returns header plus every following line indented deeper than it.
func yamlBlock(t *testing.T, doc, header string) string {
	t.Helper()
	lines := strings.Split(doc, "\n")
	indent := len(header) - len(strings.TrimLeft(header, " "))
	for i, line := range lines {
		if line != header {
			continue
		}
		out := []string{line}
		for _, next := range lines[i+1:] {
			if strings.TrimSpace(next) == "" {
				continue
			}
			if len(next)-len(strings.TrimLeft(next, " ")) <= indent {
				break
			}
			out = append(out, next)
		}
		return strings.Join(out, "\n")
	}
	t.Fatalf("no %q block in the generated document:\n%s", header, doc)
	return ""
}
