package e2e

import (
	"bytes"
	"context"
	stdsql "database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"syscall"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	ent "github.com/githonllc/entapi/internal/fixtures/httpdemo/httpdemoent"
	entapi "github.com/githonllc/entapi/runtime"
)

type problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
	Field  string `json:"field"`
}

type articlePatchService struct {
	calls int
	title string
}

func (s *articlePatchService) Patch(
	_ context.Context,
	_ *ent.Client,
	id uuid.UUID,
	_ *ent.ValidArticlePatchRequest,
) (*ent.ArticleResponse, error) {
	s.calls++
	return &ent.ArticleResponse{ID: id, Title: s.title}, nil
}

func newClient(t *testing.T) *ent.Client {
	t.Helper()
	previousErrorMap := ent.ErrorMap
	t.Cleanup(func() { ent.ErrorMap = previousErrorMap })
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := stdsql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return client
}

type testServer struct {
	*httptest.Server
	client *http.Client
	close  func()
}

func (s *testServer) Client() *http.Client { return s.client }
func (s *testServer) Close()               { s.close() }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func newTestServer(t *testing.T, handler http.Handler) *testServer {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err == nil {
		server := &httptest.Server{
			Listener: listener,
			Config:   &http.Server{Handler: handler},
		}
		server.Start()
		return &testServer{Server: server, client: server.Client(), close: server.Close}
	}
	if !errors.Is(err, syscall.EPERM) && !errors.Is(err, syscall.EACCES) {
		t.Fatalf("listen for httptest.Server: %v", err)
	}

	// Some execution sandboxes deny every socket family. Preserve the same
	// net/http request/response seam there; ordinary CI takes the real server
	// branch above.
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		response := recorder.Result()
		response.Request = request
		return response, nil
	})
	return &testServer{
		Server: &httptest.Server{URL: "http://example.com"},
		client: &http.Client{Transport: transport},
		close:  func() {},
	}
}

func request(t *testing.T, client *http.Client, method, url, contentType string, body io.Reader) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	response, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return response, data
}

func requireStatus(t *testing.T, response *http.Response, body []byte, want int) {
	t.Helper()
	if response.StatusCode != want {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, want, body)
	}
}

func panicMessage(t *testing.T, fn func()) (message string) {
	t.Helper()
	defer func() {
		value := recover()
		if value == nil {
			t.Fatal("call did not panic")
		}
		message = fmt.Sprint(value)
	}()
	fn()
	return ""
}

func createArticle(t *testing.T, server *testServer, title string) ent.ArticleResponse {
	t.Helper()
	response, body := request(t, server.Client(), http.MethodPost, server.URL+"/articles", "application/json; charset=utf-8",
		strings.NewReader(fmt.Sprintf(`{"title":%q}`, title)))
	requireStatus(t, response, body, http.StatusCreated)
	if location := response.Header.Get("Location"); location != "" {
		t.Errorf("Location = %q, want no Location header", location)
	}
	var article ent.ArticleResponse
	if err := json.Unmarshal(body, &article); err != nil {
		t.Fatalf("decode article: %v", err)
	}
	return article
}

func TestHTTPFiveEndpointsRoundTripAndBarePage(t *testing.T) {
	server := newTestServer(t, ent.API(newClient(t)))
	t.Cleanup(server.Close)

	created := createArticle(t, server, "first")
	if created.Title != "first" || created.ID == uuid.Nil {
		t.Fatalf("created = %+v", created)
	}

	response, body := request(t, server.Client(), http.MethodGet, server.URL+"/articles/"+created.ID.String(), "", nil)
	requireStatus(t, response, body, http.StatusOK)
	var got ent.ArticleResponse
	if err := json.Unmarshal(body, &got); err != nil || got.ID != created.ID {
		t.Fatalf("GET body = %s, err = %v", body, err)
	}

	response, body = request(t, server.Client(), http.MethodPatch, server.URL+"/articles/"+created.ID.String(), "application/json",
		strings.NewReader(`{"title":"updated","rank":7}`))
	requireStatus(t, response, body, http.StatusOK)
	if err := json.Unmarshal(body, &got); err != nil || got.Title != "updated" || got.Rank == nil || *got.Rank != 7 {
		t.Fatalf("PATCH body = %s, decoded = %+v, err = %v", body, got, err)
	}

	response, body = request(t, server.Client(), http.MethodGet, server.URL+"/articles?_sort=title&_page=1&_size=10", "", nil)
	requireStatus(t, response, body, http.StatusOK)
	var page map[string]json.RawMessage
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("decode page: %v", err)
	}
	if len(page) != 4 || page["data"] == nil || page["total"] == nil || page["page"] == nil || page["size"] == nil {
		t.Fatalf("page keys = %v, want exactly data,total,page,size", page)
	}

	response, body = request(t, server.Client(), http.MethodDelete, server.URL+"/articles/"+created.ID.String(), "", nil)
	requireStatus(t, response, body, http.StatusNoContent)
	if len(body) != 0 {
		t.Errorf("DELETE body = %q, want empty", body)
	}
}

func TestWithReplacementRuns(t *testing.T) {
	called := false
	api := ent.API(newClient(t)).With(ent.CreateArticleFn(func(
		context.Context,
		*ent.Client,
		*ent.ValidArticleCreateRequest,
	) (*ent.ArticleResponse, error) {
		called = true
		return &ent.ArticleResponse{Title: "replacement"}, nil
	}))
	server := newTestServer(t, api)
	t.Cleanup(server.Close)

	response, body := request(t, server.Client(), http.MethodPost, server.URL+"/articles", "application/json",
		strings.NewReader(`{"title":"input"}`))
	requireStatus(t, response, body, http.StatusCreated)
	if !called {
		t.Fatal("CreateArticleFn replacement did not run")
	}
}

func TestWithLastWinsAndVariadicEqualsChained(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*ent.APIHandler, ent.CreateArticleFn, ent.CreateArticleFn) *ent.APIHandler
	}{
		{
			name: "variadic",
			configure: func(api *ent.APIHandler, a, b ent.CreateArticleFn) *ent.APIHandler {
				return api.With(a, b)
			},
		},
		{
			name: "chained",
			configure: func(api *ent.APIHandler, a, b ent.CreateArticleFn) *ent.APIHandler {
				return api.With(a).With(b)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			last := ""
			a := ent.CreateArticleFn(func(context.Context, *ent.Client, *ent.ValidArticleCreateRequest) (*ent.ArticleResponse, error) {
				last = "a"
				return &ent.ArticleResponse{Title: "a"}, nil
			})
			b := ent.CreateArticleFn(func(context.Context, *ent.Client, *ent.ValidArticleCreateRequest) (*ent.ArticleResponse, error) {
				last = "b"
				return &ent.ArticleResponse{Title: "b"}, nil
			})
			api := ent.API(newClient(t))
			if got := tc.configure(api, a, b); got != api {
				t.Fatal("With returned a different APIHandler")
			}
			server := newTestServer(t, api)
			defer server.Close()

			response, body := request(t, server.Client(), http.MethodPost, server.URL+"/articles", "application/json",
				strings.NewReader(`{"title":"input"}`))
			requireStatus(t, response, body, http.StatusCreated)
			if last != "b" {
				t.Fatalf("replacement = %q, want b", last)
			}
		})
	}
}

func TestWithNilPanicsAtConstruction(t *testing.T) {
	t.Run("nil APIOption", func(t *testing.T) {
		message := panicMessage(t, func() {
			ent.API(newClient(t)).With(nil)
		})
		const want = "entapi: APIOption at index 0 is nil"
		if message != want {
			t.Errorf("panic = %q, want %q", message, want)
		}
	})

	t.Run("typed nil Fn", func(t *testing.T) {
		var replacement ent.CreateArticleFn
		message := panicMessage(t, func() {
			ent.API(newClient(t)).With(replacement)
		})
		const want = "entapi: CreateArticleFn is nil"
		if message != want {
			t.Errorf("panic = %q, want %q", message, want)
		}
	})
}

func TestWithAcceptsStatefulMethodValue(t *testing.T) {
	service := &articlePatchService{title: "from service"}
	api := ent.API(newClient(t)).With(ent.PatchArticleFn(service.Patch))
	server := newTestServer(t, api)
	t.Cleanup(server.Close)
	id := uuid.New()

	response, body := request(t, server.Client(), http.MethodPatch, server.URL+"/articles/"+id.String(), "application/json",
		strings.NewReader(`{"title":"input"}`))
	requireStatus(t, response, body, http.StatusOK)
	var got ent.ArticleResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if service.calls != 1 || got.Title != service.title || got.ID != id {
		t.Fatalf("service calls = %d, response = %+v", service.calls, got)
	}
}

func TestRoutesShapeOrderAndCopyIsolation(t *testing.T) {
	type routeKey struct {
		method string
		path   string
		entity string
		op     entapi.Op
	}
	want := []routeKey{
		{http.MethodGet, "/articles", "Article", entapi.OpList},
		{http.MethodPost, "/articles", "Article", entapi.OpCreate},
		{http.MethodGet, "/articles/{id}", "Article", entapi.OpGet},
		{http.MethodPatch, "/articles/{id}", "Article", entapi.OpPatch},
		{http.MethodDelete, "/articles/{id}", "Article", entapi.OpDelete},
		{http.MethodGet, "/audit_logs", "AuditLog", entapi.OpList},
		{http.MethodPost, "/audit_logs", "AuditLog", entapi.OpCreate},
		{http.MethodGet, "/audit_logs/{id}", "AuditLog", entapi.OpGet},
		{http.MethodPatch, "/audit_logs/{id}", "AuditLog", entapi.OpPatch},
		// The generated document, last: it is appended after every entity's
		// routes, so its position is a fact about registration order rather
		// than an accident of the schema (#76). It belongs to no resource, so
		// Entity and Op are the zero value -- that is what makes "every route
		// with an Op" a usable way to say "the resource surface".
		{http.MethodGet, "/openapi.yaml", "", entapi.OpNone},
	}
	api := ent.API(newClient(t))
	routes := api.Routes()
	if len(routes) != len(want) {
		t.Fatalf("route count = %d, want %d", len(routes), len(want))
	}
	legalityProbe := http.NewServeMux()
	for i, route := range routes {
		if route.Handler == nil {
			t.Fatalf("route %d has a nil Handler", i)
		}
		if got := (routeKey{route.Method, route.Path, route.Entity, route.Op}); got != want[i] {
			t.Errorf("route %d = %+v, want %+v", i, got, want[i])
		}
		legalityProbe.Handle(route.Method+" "+route.Path, route.Handler)
	}

	routes[0] = entapi.Route{
		Method: http.MethodPost,
		Path:   "/poison",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}),
	}
	if fresh := api.Routes()[0]; fresh.Method != http.MethodGet || fresh.Path != "/articles" {
		t.Fatalf("mutating Routes result changed next result: %+v", fresh)
	}
	mux := http.NewServeMux()
	api.Mount(mux)
	server := newTestServer(t, mux)
	t.Cleanup(server.Close)
	response, body := request(t, server.Client(), http.MethodGet, server.URL+"/articles", "", nil)
	requireStatus(t, response, body, http.StatusOK)
}

// TestRouteIdentitySplitsTheManifestWithoutPathMatching is the reason
// Route.Entity and Route.Op exist. Splitting the surface by audience -- mount
// AuditLog behind an internal-only middleware, expose Article publicly, wrap
// every write with an audit log -- used to require matching on Path text, where
// a typo produces a selector that matches nothing and reports nothing.
//
// The assertions below are deliberately written the way a consumer would write
// the routing itself, so the test fails for the same reason a consumer's split
// would break.
func TestRouteIdentitySplitsTheManifestWithoutPathMatching(t *testing.T) {
	routes := ent.API(newClient(t)).Routes()

	var public, internal, writes, unowned []entapi.Route
	for _, route := range routes {
		switch route.Entity {
		case "Article":
			public = append(public, route)
		case "AuditLog":
			internal = append(internal, route)
		case "":
			unowned = append(unowned, route)
		default:
			t.Errorf("route %s %s carries unknown entity %q", route.Method, route.Path, route.Entity)
		}
		switch route.Op {
		case entapi.OpCreate, entapi.OpPatch, entapi.OpDelete:
			writes = append(writes, route)
		}
	}

	// Article is a full resource; AuditLog Excepts delete (see its schema), so
	// the split has to come out asymmetric or it is not reading real identity.
	if len(public) != 5 {
		t.Errorf("Article routes = %d, want 5", len(public))
	}
	if len(internal) != 4 {
		t.Errorf("AuditLog routes = %d, want 4 (delete is Excepted)", len(internal))
	}
	if len(writes) != 5 {
		t.Errorf("write routes = %d, want 5 (3 Article + 2 AuditLog)", len(writes))
	}
	// Exactly one route belongs to no resource, and it is the document.
	if len(unowned) != 1 || unowned[0].Path != "/openapi.yaml" {
		t.Errorf("routes with no entity = %+v, want only GET /openapi.yaml", unowned)
	}
	for _, route := range unowned {
		if route.Op != entapi.OpNone {
			t.Errorf("%s %s has no entity but carries Op %q", route.Method, route.Path, route.Op)
		}
	}

	// Every resource route carries both halves of its identity: an Op without
	// an Entity, or the reverse, would make one of the two splits above silently
	// incomplete rather than loudly wrong.
	for _, route := range routes {
		if (route.Entity == "") != (route.Op == entapi.OpNone) {
			t.Errorf("%s %s has a half-filled identity: entity=%q op=%q",
				route.Method, route.Path, route.Entity, route.Op)
		}
	}
}

func TestMountMatchesRoutes(t *testing.T) {
	api := ent.API(newClient(t))
	mux := http.NewServeMux()
	api.Mount(mux)
	missingID := "00000000-0000-0000-0000-000000000001"

	newRouteRequest := func(route entapi.Route, direct bool) *http.Request {
		t.Helper()
		path := strings.ReplaceAll(route.Path, "{id}", missingID)
		var body io.Reader
		if route.Method == http.MethodPost || route.Method == http.MethodPatch {
			body = strings.NewReader(`{}`)
		}
		request := httptest.NewRequest(route.Method, path, body)
		if body != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		if direct && strings.Contains(route.Path, "{id}") {
			request.SetPathValue("id", missingID)
		}
		return request
	}

	for _, route := range api.Routes() {
		mounted := httptest.NewRecorder()
		mux.ServeHTTP(mounted, newRouteRequest(route, false))
		direct := httptest.NewRecorder()
		route.Handler.ServeHTTP(direct, newRouteRequest(route, true))
		if mounted.Code != direct.Code {
			t.Errorf("%s %s: mounted status = %d, direct status = %d",
				route.Method, route.Path, mounted.Code, direct.Code)
		}
	}

	source, err := os.ReadFile("../httpdemoent/entapi_http.go")
	if err != nil {
		t.Fatal(err)
	}
	want := "func (h *APIHandler) Mount(mux *http.ServeMux) {\n\tfor _, rt := range h.routes {\n\t\tmux.Handle(rt.Method+\" \"+rt.Path, rt.Handler)\n\t}\n}"
	if !strings.Contains(string(source), want) {
		t.Fatal("Mount is not a literal range walk over h.routes")
	}
}

func TestRoutesSupportThirdPartyRouterAdapterMechanics(t *testing.T) {
	if got := strings.ReplaceAll("/articles/{id}", "{id}", ":id"); got != "/articles/:id" {
		t.Fatalf("translated path = %q, want /articles/:id", got)
	}
	client := newClient(t)
	article, err := client.Article.Create().SetTitle("adapter").Save(context.Background())
	if err != nil {
		t.Fatalf("create article: %v", err)
	}
	api := ent.API(client)
	var getArticle entapi.Route
	for _, route := range api.Routes() {
		if route.Method == http.MethodGet && route.Path == "/articles/{id}" {
			getArticle = route
			break
		}
	}
	if getArticle.Handler == nil {
		t.Fatal("GET /articles/{id} route not found")
	}

	request := httptest.NewRequest(http.MethodGet, "/articles/:id", nil)
	request.SetPathValue("id", article.ID.String())
	recorder := httptest.NewRecorder()
	getArticle.Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	stdlibMux := http.NewServeMux()
	stdlibMux.HandleFunc("GET /articles/{id}", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.PathValue("id"))
	})
	encoded := httptest.NewRequest(http.MethodGet, "/articles/a%2Fb", nil)
	if encoded.URL.Path != "/articles/a/b" || encoded.URL.RawPath != "/articles/a%2Fb" {
		t.Fatalf("encoded request Path = %q, RawPath = %q", encoded.URL.Path, encoded.URL.RawPath)
	}
	encodedRecorder := httptest.NewRecorder()
	stdlibMux.ServeHTTP(encodedRecorder, encoded)
	if encodedRecorder.Code != http.StatusOK || encodedRecorder.Body.String() != "a/b" {
		t.Fatalf("stdlib encoded segment: status = %d, id = %q", encodedRecorder.Code, encodedRecorder.Body.String())
	}
}

func TestRoutesSupportSelectiveMethodWrapping(t *testing.T) {
	api := ent.API(newClient(t))
	mux := http.NewServeMux()
	requireAuth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
	for _, route := range api.Routes() {
		handler := route.Handler
		if route.Method == http.MethodDelete {
			handler = requireAuth(handler)
		}
		mux.Handle(route.Method+" "+route.Path, handler)
	}
	server := newTestServer(t, mux)
	t.Cleanup(server.Close)

	response, body := request(t, server.Client(), http.MethodGet, server.URL+"/articles", "", nil)
	requireStatus(t, response, body, http.StatusOK)
	response, body = request(t, server.Client(), http.MethodDelete,
		server.URL+"/articles/00000000-0000-0000-0000-000000000001", "", nil)
	requireStatus(t, response, body, http.StatusUnauthorized)
}

func TestWithAfterRoutesAndMountAppliesBeforeRequest(t *testing.T) {
	api := ent.API(newClient(t))
	before := api.Routes()
	mux := http.NewServeMux()
	api.Mount(mux)
	called := false
	api.With(ent.CreateArticleFn(func(context.Context, *ent.Client, *ent.ValidArticleCreateRequest) (*ent.ArticleResponse, error) {
		called = true
		return &ent.ArticleResponse{Title: "after mount"}, nil
	}))
	if after := api.Routes(); len(after) != len(before) {
		t.Fatalf("route count changed from %d to %d", len(before), len(after))
	}
	server := newTestServer(t, mux)
	t.Cleanup(server.Close)

	response, body := request(t, server.Client(), http.MethodPost, server.URL+"/articles", "application/json",
		strings.NewReader(`{"title":"input"}`))
	requireStatus(t, response, body, http.StatusCreated)
	if !called {
		t.Fatal("replacement installed after Routes and Mount did not run")
	}
}

func TestHTTPProblemStatuses(t *testing.T) {
	server := newTestServer(t, ent.API(newClient(t)))
	t.Cleanup(server.Close)

	tests := []struct {
		name        string
		method      string
		path        string
		contentType string
		body        io.Reader
		status      int
		field       string
	}{
		{"413 body cap", http.MethodPost, "/articles", "application/json", bytes.NewReader(bytes.Repeat([]byte("x"), 1<<20+1)), http.StatusRequestEntityTooLarge, ""},
		{"415 missing content type", http.MethodPost, "/articles", "", strings.NewReader(`{"title":"x"}`), http.StatusUnsupportedMediaType, ""},
		{"400 unknown field names field", http.MethodPost, "/articles", "application/json", strings.NewReader(`{"title":"x","mystery":1}`), http.StatusBadRequest, "mystery"},
		{"400 syntax has no field", http.MethodPost, "/articles", "application/json", strings.NewReader(`{"title":`), http.StatusBadRequest, ""},
		{"400 malformed path id", http.MethodGet, "/articles/not-a-uuid", "", nil, http.StatusBadRequest, ""},
		{"400 query bind", http.MethodGet, "/articles?unknown=value", "", nil, http.StatusBadRequest, ""},
		{"400 middle sort allow-list", http.MethodGet, "/articles?_sort=secret", "", nil, http.StatusBadRequest, ""},
		{"404 missing row", http.MethodGet, "/articles/00000000-0000-0000-0000-000000000001", "", nil, http.StatusNotFound, ""},
		{"422 validation", http.MethodPost, "/articles", "application/json", strings.NewReader(`{}`), http.StatusUnprocessableEntity, ""},
		{"422 save validation names field", http.MethodPost, "/articles", "application/json", strings.NewReader(`{"title":"` + strings.Repeat("x", 65) + `"}`), http.StatusUnprocessableEntity, "title"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			response, body := request(t, server.Client(), tc.method, server.URL+tc.path, tc.contentType, tc.body)
			requireStatus(t, response, body, tc.status)
			if got := response.Header.Get("Content-Type"); got != "application/problem+json" {
				t.Errorf("Content-Type = %q, want application/problem+json", got)
			}
			var got problem
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("decode problem: %v; body = %s", err, body)
			}
			if got.Type != "about:blank" || got.Status != tc.status || got.Detail == "" || got.Field != tc.field {
				t.Errorf("problem = %+v, want status %d field %q", got, tc.status, tc.field)
			}
			if tc.path == "/articles/not-a-uuid" && !strings.Contains(got.Detail, "not-a-uuid") {
				t.Errorf("path-id detail = %q, want the invalid id", got.Detail)
			}
		})
	}

	t.Run("409 unique violation", func(t *testing.T) {
		createArticle(t, server, "duplicate")
		response, body := request(t, server.Client(), http.MethodPost, server.URL+"/articles", "application/json", strings.NewReader(`{"title":"duplicate"}`))
		requireStatus(t, response, body, http.StatusConflict)
	})
}

func TestHTTPUnknownDialectLeavesDuplicateAs500(t *testing.T) {
	previousErrorMap := ent.ErrorMap
	t.Cleanup(func() { ent.ErrorMap = previousErrorMap })

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := stdsql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	sqliteClient := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	if err := sqliteClient.Schema.Create(context.Background()); err != nil {
		t.Fatalf("migrate through SQLite dialect: %v", err)
	}
	client := ent.NewClient(ent.Driver(entsql.OpenDB("weird", db)))
	t.Cleanup(func() { _ = client.Close() })

	server := newTestServer(t, ent.API(client))
	t.Cleanup(server.Close)
	createArticle(t, server, "duplicate")
	response, body := request(t, server.Client(), http.MethodPost, server.URL+"/articles", "application/json",
		strings.NewReader(`{"title":"duplicate"}`))
	requireStatus(t, response, body, http.StatusInternalServerError)
}

func TestHTTPUniqueViolationEscapeHatchOrdering(t *testing.T) {
	custom := func(error) bool { return false }

	t.Run("installed before API survives", func(t *testing.T) {
		client := newClient(t)
		ent.ErrorMap = ent.ErrorMap.WithUniqueViolation(custom)
		server := newTestServer(t, ent.API(client))
		defer server.Close()
		createArticle(t, server, "before")
		response, body := request(t, server.Client(), http.MethodPost, server.URL+"/articles", "application/json",
			strings.NewReader(`{"title":"before"}`))
		requireStatus(t, response, body, http.StatusInternalServerError)
	})

	t.Run("installed after API overrides automatic", func(t *testing.T) {
		client := newClient(t)
		api := ent.API(client)
		if !ent.ErrorMap.HasUniqueViolation() {
			t.Fatal("API did not install the SQLite uniqueness determination")
		}
		ent.ErrorMap = ent.ErrorMap.WithUniqueViolation(custom)
		server := newTestServer(t, api)
		defer server.Close()
		createArticle(t, server, "after")
		response, body := request(t, server.Client(), http.MethodPost, server.URL+"/articles", "application/json",
			strings.NewReader(`{"title":"after"}`))
		requireStatus(t, response, body, http.StatusInternalServerError)
	})
}

func TestHTTPUnclassifiedMiddleErrorIs500(t *testing.T) {
	client := newClient(t)
	server := newTestServer(t, ent.API(client))
	t.Cleanup(server.Close)
	if err := client.Close(); err != nil {
		t.Fatalf("close client: %v", err)
	}

	response, body := request(t, server.Client(), http.MethodGet, server.URL+"/articles", "", nil)
	requireStatus(t, response, body, http.StatusInternalServerError)
}

func TestHTTPRejectsImmutablePatchFieldByGeneratedTagData(t *testing.T) {
	server := newTestServer(t, ent.API(newClient(t)))
	t.Cleanup(server.Close)
	article := createArticle(t, server, "immutable")

	response, body := request(t, server.Client(), http.MethodPatch, server.URL+"/articles/"+article.ID.String(), "application/json",
		strings.NewReader(`{"slug":"changed"}`))
	requireStatus(t, response, body, http.StatusBadRequest)
	var got problem
	if err := json.Unmarshal(body, &got); err != nil || got.Field != "slug" {
		t.Fatalf("problem = %+v, err = %v, want field slug", got, err)
	}
}

func TestHTTPDirectMountStripPrefixActorAndShortCircuit(t *testing.T) {
	client := newClient(t)

	t.Run("Mount", func(t *testing.T) {
		mux := http.NewServeMux()
		ent.API(client).Mount(mux)
		server := newTestServer(t, mux)
		defer server.Close()
		response, body := request(t, server.Client(), http.MethodGet, server.URL+"/articles", "", nil)
		requireStatus(t, response, body, http.StatusOK)
	})

	t.Run("StripPrefix", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.Handle("/v1/", http.StripPrefix("/v1", ent.API(client)))
		server := newTestServer(t, mux)
		defer server.Close()
		response, body := request(t, server.Client(), http.MethodGet, server.URL+"/v1/articles", "", nil)
		requireStatus(t, response, body, http.StatusOK)
	})

	t.Run("actor", func(t *testing.T) {
		mux := http.NewServeMux()
		ent.API(client).Mount(mux)
		mux.HandleFunc("GET /actor", func(w http.ResponseWriter, r *http.Request) {
			actor, ok := entapi.ActorFrom(r.Context())
			if !ok {
				http.Error(w, "missing actor", http.StatusInternalServerError)
				return
			}
			_, _ = fmt.Fprint(w, actor)
		})
		withActor := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mux.ServeHTTP(w, r.WithContext(entapi.WithActor(r.Context(), "alice")))
		})
		server := newTestServer(t, withActor)
		defer server.Close()
		response, body := request(t, server.Client(), http.MethodGet, server.URL+"/actor", "", nil)
		requireStatus(t, response, body, http.StatusOK)
		if string(body) != "alice" {
			t.Errorf("actor body = %q, want alice", body)
		}
	})

	t.Run("middleware short-circuits before binding", func(t *testing.T) {
		api := ent.API(client)
		requireAuth := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = next
				entapi.WriteProblem(w, http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized), fmt.Errorf("authentication required"))
			})
		}
		server := newTestServer(t, requireAuth(api))
		defer server.Close()
		response, body := request(t, server.Client(), http.MethodPost, server.URL+"/articles", "application/json", strings.NewReader(`{"certainly":`))
		requireStatus(t, response, body, http.StatusUnauthorized)
	})
}

func TestHTTPRouterResidueIsStdlibPlainText(t *testing.T) {
	server := newTestServer(t, ent.API(newClient(t)))
	t.Cleanup(server.Close)

	t.Run("404 unmatched path", func(t *testing.T) {
		response, body := request(t, server.Client(), http.MethodGet, server.URL+"/missing", "", nil)
		requireStatus(t, response, body, http.StatusNotFound)
		if !strings.HasPrefix(response.Header.Get("Content-Type"), "text/plain") {
			t.Errorf("router-level 404 Content-Type = %q, want stdlib text/plain residue", response.Header.Get("Content-Type"))
		}
	})

	t.Run("405 Excepted delete with Allow", func(t *testing.T) {
		response, body := request(t, server.Client(), http.MethodDelete,
			server.URL+"/audit_logs/00000000-0000-0000-0000-000000000001", "", nil)
		requireStatus(t, response, body, http.StatusMethodNotAllowed)
		if response.Header.Get("Allow") == "" {
			t.Error("405 response has no Allow header")
		}
		if !strings.HasPrefix(response.Header.Get("Content-Type"), "text/plain") {
			t.Errorf("router-level 405 Content-Type = %q, want stdlib text/plain residue", response.Header.Get("Content-Type"))
		}
	})
}
