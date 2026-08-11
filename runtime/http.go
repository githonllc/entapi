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

// Op names the CRUD operation an Endpoint implements. It is a distinct type
// rather than a bare string so that selecting a subset of the manifest —
// "everything on AuditLog", "every write" — is a comparison the compiler checks.
// A misspelled string literal matches nothing and reports nothing, which is
// exactly the failure that selecting by Path prefix already has.
//
// It deliberately does NOT reuse api.Op. That package imports
// entgo.io/ent/schema, and TestRuntimePackageIsGeneratorFree pins this
// package's transitive entgo.io count at zero; importing api to save a
// declaration would trade the whole runtime/generator seam for it. The values
// are the same strings, and the generator writes them from its own api.Op.
type Op string

// The operations a generated Endpoint can carry. OpNone is the zero value, held
// by endpoints that are not part of the resource surface — today only
// GET /openapi.yaml.
const (
	OpNone   Op = ""
	OpList   Op = "list"
	OpCreate Op = "create"
	OpGet    Op = "get"
	OpPatch  Op = "patch"
	OpDelete Op = "delete"
)

// Endpoint is the contract record of one generated HTTP handler: it is data a
// consumer composes into their own router, not routing machinery. entapi
// registers nothing anywhere; the internal ServeMux behind the generated
// ServeHTTP and Mount is an optional convenience built from this same manifest.
//
// Method and Path are the handler's signature. Path is load-bearing rather than
// descriptive: its {name} placeholders fix the path-parameter names the handler
// body reads back through r.PathValue, which is why Bind takes those names from
// Path instead of hardcoding any.
//
// Entity and Op carry the identity the path used to hide: they let a consumer
// split the manifest by audience, wrap only writes, or drop one entity's
// surface without matching on Path text. Both are empty for an endpoint that
// belongs to no resource.
//
// Endpoint is NOT comparable, because Handler holds a func value at run time:
// ep1 == ep2, a map[Endpoint]V and slices.Contains over a []Endpoint each panic
// with "comparing uncomparable type http.HandlerFunc" — at run time, since the
// interface field makes it compile. Deduplicating or set-testing the manifest
// therefore needs a key built from the comparable fields, Method and Path
// (adding Entity and Op where the same route is reached more than one way).
type Endpoint struct {
	Method  string
	Path    string
	Handler http.Handler

	// Entity is the Ent type name this endpoint was generated for, e.g. "Article".
	Entity string
	// Op is the CRUD operation this endpoint implements.
	Op Op
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
