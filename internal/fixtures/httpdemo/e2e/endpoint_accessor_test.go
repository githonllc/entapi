package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	ent "github.com/githonllc/entapi/internal/fixtures/httpdemo/httpdemoent"
	entapi "github.com/githonllc/entapi/runtime"
)

// TestEndpointAccessorsAreTheManifestTakenByName is the run-time half of #119's
// consistency claim. The generator's own test proves the manifest is built from
// the accessors in the rendered template; this proves the built handler a
// consumer holds answers both questions with the same set.
//
// The accessor set is collected by SIGNATURE, not by name: every exported
// *APIHandler method that takes nothing and returns an entapi.Endpoint. A name
// filter would make the test agree with the template's naming convention by
// construction, and an accessor renamed on one side only would still pass.
func TestEndpointAccessorsAreTheManifestTakenByName(t *testing.T) {
	api := ent.API(newClient(t))

	type endpointKey struct {
		method string
		path   string
		entity string
		op     entapi.Op
	}
	keyOf := func(e entapi.Endpoint) endpointKey {
		return endpointKey{e.Method, e.Path, e.Entity, e.Op}
	}

	byKey := map[endpointKey]string{}
	handlerType := reflect.TypeOf(api)
	endpointType := reflect.TypeOf(entapi.Endpoint{})
	for i := range handlerType.NumMethod() {
		method := handlerType.Method(i)
		if method.Type.NumIn() != 1 || method.Type.NumOut() != 1 || method.Type.Out(0) != endpointType {
			continue
		}
		endpoint := method.Func.Call([]reflect.Value{reflect.ValueOf(api)})[0].Interface().(entapi.Endpoint)
		if endpoint.Handler == nil {
			t.Errorf("%s returns an endpoint with a nil Handler", method.Name)
		}
		if previous, duplicate := byKey[keyOf(endpoint)]; duplicate {
			t.Errorf("%s and %s both return %s %s; one endpoint must have exactly one name",
				previous, method.Name, endpoint.Method, endpoint.Path)
		}
		byKey[keyOf(endpoint)] = method.Name
	}

	endpoints := api.Endpoints()
	if len(byKey) != len(endpoints) {
		t.Fatalf("%d endpoint accessors for %d manifest entries: %v vs %v", len(byKey), len(endpoints), byKey, endpoints)
	}
	for _, endpoint := range endpoints {
		name, ok := byKey[keyOf(endpoint)]
		if !ok {
			t.Errorf("manifest entry %s %s (entity %q, op %q) has no accessor",
				endpoint.Method, endpoint.Path, endpoint.Entity, endpoint.Op)
			continue
		}
		switch {
		case endpoint.Op == entapi.OpNone && name != "OpenAPIEndpoint":
			t.Errorf("the manifest's Op-less entry %s %s is reached by %s, want OpenAPIEndpoint",
				endpoint.Method, endpoint.Path, name)
		case endpoint.Op != entapi.OpNone && name == "OpenAPIEndpoint":
			t.Errorf("OpenAPIEndpoint returns %s %s, which carries op %q", endpoint.Method, endpoint.Path, endpoint.Op)
		}
	}

	// AuditLog carries Except(api.OpDelete), so an accessor set that ignored
	// Except would be one larger than the manifest -- and would compile.
	if _, ok := reflect.TypeOf(api).MethodByName("DeleteAuditLogEndpoint"); ok {
		t.Error("DeleteAuditLogEndpoint exists despite Except(api.OpDelete)")
	}
}

// TestEndpointTakenByNameServesLaterWithReplacement pins the design constraint
// that makes an accessor safe to take at wiring time: the endpoint carries a
// handler that reads the implementation through the receiver, so it is not a
// snapshot. An accessor that captured the function value instead would pass
// every shape assertion above and silently serve the default forever.
func TestEndpointTakenByNameServesLaterWithReplacement(t *testing.T) {
	api := ent.API(newClient(t))
	taken := api.CreateArticleEndpoint()

	mux := http.NewServeMux()
	mux.Handle(taken.Method+" "+taken.Path, taken.Handler)
	server := newTestServer(t, mux)
	t.Cleanup(server.Close)

	called := false
	api.With(ent.CreateArticleFn(func(context.Context, *ent.Client, *ent.ValidArticleCreateRequest) (*ent.ArticleResponse, error) {
		called = true
		return &ent.ArticleResponse{Title: "replacement"}, nil
	}))

	response, body := request(t, server.Client(), http.MethodPost, server.URL+taken.Path, "application/json",
		strings.NewReader(`{"title":"input"}`))
	requireStatus(t, response, body, http.StatusCreated)
	if !called {
		t.Fatal("an endpoint taken by name before With served the default implementation")
	}
}

// TestEndpointAccessorServesRemappedAndSelectivePaths is the composition ladder's
// middle rung on a real server: two accessors, one router the consumer owns, one
// of them remapped through Bind to a path the generator never emitted -- and
// nothing else exposed.
func TestEndpointAccessorServesRemappedAndSelectivePaths(t *testing.T) {
	client := newClient(t)
	article, err := client.Article.Create().SetTitle("pinned").Save(context.Background())
	if err != nil {
		t.Fatalf("seed article: %v", err)
	}
	api := ent.API(client)

	list := api.ListArticlesEndpoint()
	get := api.GetArticleEndpoint()

	mux := http.NewServeMux()
	mux.Handle(list.Method+" /v1/articles", list.Handler)
	// The pinned-id pattern from the README: Bind feeds the endpoint's own
	// placeholder names, so the path carries none of its own.
	mux.Handle(get.Method+" /v1/featured", get.Bind(func(string) string { return article.ID.String() }))
	server := newTestServer(t, mux)
	t.Cleanup(server.Close)

	response, body := request(t, server.Client(), http.MethodGet, server.URL+"/v1/featured", "", nil)
	requireStatus(t, response, body, http.StatusOK)
	var got ent.ArticleResponse
	if err := json.Unmarshal(body, &got); err != nil || got.ID != article.ID {
		t.Fatalf("GET /v1/featured body = %s, decoded = %+v, err = %v", body, got, err)
	}

	response, body = request(t, server.Client(), http.MethodGet, server.URL+"/v1/articles", "", nil)
	requireStatus(t, response, body, http.StatusOK)

	// Selective exposure: only the two accessors registered above are served, so
	// the entity whose endpoints were never taken is unreachable.
	response, body = request(t, server.Client(), http.MethodGet, server.URL+"/audit_logs", "", nil)
	requireStatus(t, response, body, http.StatusNotFound)
}
