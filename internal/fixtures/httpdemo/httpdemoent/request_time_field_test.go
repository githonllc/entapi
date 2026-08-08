// This file is hand-written. Generation owns the package around it.
package httpdemoent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	entapi "github.com/githonllc/entapi/runtime"
)

func TestAPIHandlerReadsFunctionFieldAtRequestTime(t *testing.T) {
	h := API(nil)
	called := false
	h.createArticle = func(ctx context.Context, _ *Client, _ *ValidArticleCreateRequest) (*ArticleResponse, error) {
		called = true
		actor, ok := entapi.ActorFrom(ctx)
		if !ok || actor != "tester" {
			t.Errorf("actor = %v, %v, want tester, true", actor, ok)
		}
		return &ArticleResponse{Title: "marker"}, nil
	}

	request := httptest.NewRequest(http.MethodPost, "/articles", strings.NewReader(`{"title":"input"}`))
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	request = request.WithContext(entapi.WithActor(request.Context(), "tester"))
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, request)

	if !called {
		t.Fatal("marker closure assigned after API returned was not called")
	}
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
}
