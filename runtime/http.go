package entapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// FieldError associates an error with the request field that caused it.
type FieldError struct {
	Field string
	Err   error
}

// Error implements error.
func (e *FieldError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Err == nil {
		return e.Field
	}
	if e.Field == "" {
		return e.Err.Error()
	}
	return fmt.Sprintf("%s: %v", e.Field, e.Err)
}

// Unwrap returns the underlying field error.
func (e *FieldError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Route describes one generated stdlib HTTP route.
type Route struct {
	Method  string
	Path    string
	Handler http.Handler
}

type problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
	Field  string `json:"field,omitempty"`
}

// WriteProblem writes an RFC 9457 application/problem+json response.
func WriteProblem(w http.ResponseWriter, status int, title string, err error) {
	p := problem{
		Type:   "about:blank",
		Title:  title,
		Status: status,
	}
	if err != nil {
		p.Detail = err.Error()
	}
	var fieldErr *FieldError
	if errors.As(err, &fieldErr) {
		p.Field = fieldErr.Field
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(p)
}

type actorContextKey struct{}

// WithActor returns a child context carrying the authenticated actor.
func WithActor(ctx context.Context, actor any) context.Context {
	return context.WithValue(ctx, actorContextKey{}, actor)
}

// ActorFrom returns the authenticated actor stored in ctx.
func ActorFrom(ctx context.Context) (any, bool) {
	actor := ctx.Value(actorContextKey{})
	return actor, actor != nil
}
