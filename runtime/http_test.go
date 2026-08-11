package entapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestWriteProblemWritesRFC9457Document(t *testing.T) {
	recorder := httptest.NewRecorder()
	err := errors.New("database unavailable")

	WriteProblem(recorder, http.StatusInternalServerError, "Internal Server Error", err)

	if got := recorder.Code; got != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", got, http.StatusInternalServerError)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", got)
	}
	want := "{\"type\":\"about:blank\",\"title\":\"Internal Server Error\",\"status\":500,\"detail\":\"database unavailable\"}\n"
	if got := recorder.Body.String(); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestWriteProblemIncludesFieldFromWrappedFieldError(t *testing.T) {
	recorder := httptest.NewRecorder()
	cause := errors.New("is required")
	err := fmt.Errorf("binding failed: %w", &FieldError{Field: "title", Err: cause})

	WriteProblem(recorder, http.StatusUnprocessableEntity, "Unprocessable Entity", err)

	want := "{\"type\":\"about:blank\",\"title\":\"Unprocessable Entity\",\"status\":422,\"detail\":\"binding failed: title: is required\",\"field\":\"title\"}\n"
	if got := recorder.Body.String(); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
	if !errors.Is(err, cause) {
		t.Error("FieldError does not unwrap to its cause")
	}
}

func TestActorContextRoundTrip(t *testing.T) {
	type actor struct{ ID string }
	want := actor{ID: "user-7"}
	ctx := WithActor(context.Background(), want)

	got, ok := ActorFrom(ctx)
	if !ok {
		t.Fatal("ActorFrom reports no actor")
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("actor = %#v, want %#v", got, want)
	}

	if _, ok := ActorFrom(context.Background()); ok {
		t.Error("ActorFrom reports an actor for a context without one")
	}
}

func TestEndpointCarriesStdlibHandler(t *testing.T) {
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	endpoint := Endpoint{Method: http.MethodGet, Path: "/articles", Handler: handler}

	if endpoint.Method != http.MethodGet || endpoint.Path != "/articles" || endpoint.Handler == nil {
		t.Errorf("endpoint = %+v", endpoint)
	}
}
