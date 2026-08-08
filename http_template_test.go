package entapi

import (
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
		"type APIHandler struct {",
		"createProbe CreateProbeFn",
		"createProbe: CreateProbe,",
		`Path:    "/probes"`,
		`Path:    "/probes/{id}"`,
		"Handler: http.HandlerFunc(h.handleCreateProbe)",
		"func API(client *Client) *APIHandler",
		"func (h *APIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request)",
		"for _, rt := range h.routes {\n\t\tmux.Handle(rt.Method+\" \"+rt.Path, rt.Handler)\n\t}",
	)
}

func assertSourceContains(t *testing.T, src string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(src, want) {
			t.Errorf("generated source does not contain %q\n%s", want, src)
		}
	}
}
