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
