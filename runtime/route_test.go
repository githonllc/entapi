package entapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestColonPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "placeholder", path: "/users/{id}", want: "/users/:id"},
		{name: "no placeholder", path: "/users", want: "/users"},
		{name: "root", path: "/", want: "/"},
		{name: "empty", path: "", want: ""},
		{name: "multiple placeholders", path: "/a/{x}/b/{y}", want: "/a/:x/b/:y"},
		{name: "partial segment", path: "/a{b}c", want: "/a{b}c"},
		{name: "empty placeholder name", path: "/{}", want: "/{}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ColonPath(tt.path); got != tt.want {
				t.Errorf("ColonPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

type routeTestHandler struct {
	serve func(http.ResponseWriter, *http.Request)
}

func (h *routeTestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.serve != nil {
		h.serve(w, r)
	}
}

func TestRouteBindWithoutPlaceholderReturnsHandlerUnchanged(t *testing.T) {
	handler := &routeTestHandler{}
	route := Route{Path: "/users", Handler: handler}

	got := route.Bind(func(string) string {
		t.Fatal("get called for a route without placeholders")
		return ""
	})

	if got != handler {
		t.Fatalf("Bind returned handler %T at %p, want original handler %T at %p", got, got, handler, handler)
	}
}

func TestRouteBindSetsPathValue(t *testing.T) {
	observed := ""
	handler := &routeTestHandler{serve: func(_ http.ResponseWriter, r *http.Request) {
		observed = r.PathValue("id")
	}}
	route := Route{Path: "/users/{id}", Handler: handler}
	get := func(key string) string {
		return map[string]string{"id": "42"}[key]
	}

	route.Bind(get).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/users/42", nil))

	if observed != "42" {
		t.Errorf("PathValue(%q) = %q, want %q", "id", observed, "42")
	}
}

func TestRouteBindPassesPlaceholderNameToGet(t *testing.T) {
	var gotKey string
	calls := 0
	route := Route{
		Path:    "/users/{id}",
		Handler: &routeTestHandler{},
	}

	route.Bind(func(key string) string {
		calls++
		gotKey = key
		return "42"
	}).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/users/42", nil))

	if calls != 1 || gotKey != "id" {
		t.Errorf("get calls = %d with key %q, want one call with key %q", calls, gotKey, "id")
	}
}

func TestRouteBindSetsMultiplePathValues(t *testing.T) {
	values := map[string]string{}
	var gotKeys []string
	handler := &routeTestHandler{serve: func(_ http.ResponseWriter, r *http.Request) {
		values["x"] = r.PathValue("x")
		values["y"] = r.PathValue("y")
	}}
	route := Route{Path: "/a/{x}/b/{y}", Handler: handler}
	wantValues := map[string]string{"x": "first", "y": "second"}

	route.Bind(func(key string) string {
		gotKeys = append(gotKeys, key)
		return wantValues[key]
	}).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/a/first/b/second", nil))

	if values["x"] != "first" || values["y"] != "second" {
		t.Errorf("path values = %#v, want %#v", values, wantValues)
	}
	if len(gotKeys) != 2 || gotKeys[0] != "x" || gotKeys[1] != "y" {
		t.Errorf("get keys = %#v, want []string{%q, %q}", gotKeys, "x", "y")
	}
}

func TestRouteBindSetsEmptyPathValueAndDelegates(t *testing.T) {
	innerRan := false
	observed := "not empty"
	getCalled := false
	handler := &routeTestHandler{serve: func(_ http.ResponseWriter, r *http.Request) {
		innerRan = true
		observed = r.PathValue("id")
	}}
	route := Route{Path: "/users/{id}", Handler: handler}

	route.Bind(func(string) string {
		getCalled = true
		return ""
	}).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/users/", nil))

	if !getCalled {
		t.Error("get was not called")
	}
	if !innerRan {
		t.Error("inner handler did not run")
	}
	if observed != "" {
		t.Errorf("PathValue(%q) = %q, want empty string", "id", observed)
	}
}
